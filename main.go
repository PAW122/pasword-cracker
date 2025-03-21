package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"ps/targets"
)

// --------------------------------------
// KONFIGURACJA / ZMIENNE GLOBALNE
// --------------------------------------

var (
	generatedStringLength = 3
	targetObj             targets.Target
	foundFlag             int32
	finalString           string
	processed             int64
	currentIndex          int64
	checkpointInterval    = 5 * time.Second
)

// Zbiór znaków do generowania haseł
var characters = []rune("abcdefghijklmnopqrstuvwxyz0123456789@ABCDEFGHIJKLMNOPQRSTUVWXYZ")

const checkpointFile = "checkpoint.txt"

// --------------------------------------
// FUNKCJA MAIN
// --------------------------------------
func main() {
	// Parsowanie flag
	threadsFlag := flag.Int("threads", runtime.NumCPU(), "Liczba wątków (goroutines)")
	resumeFlag := flag.Bool("resume", false, "Czy wznowić od zapisanego checkpointu")
	targetTypeFlag := flag.String("type", "", "Rodzaj targetu: 'string' lub 'zip'")
	targetValueFlag := flag.String("target", "", "Docelowy ciąg znaków lub ścieżka do pliku .zip")
	flag.Parse()

	// Pobranie wartości targetu
	targetInput := *targetValueFlag
	if targetInput == "" && flag.NArg() > 0 {
		targetInput = flag.Arg(0)
	}

	// Wykrywanie trybu targetu
	targetMode := detectTargetMode(*targetTypeFlag, targetInput)

	// Inicjalizacja targetu
	var err error
	if targetMode == "zip" {
		targetObj, err = targets.NewZipTarget(targetInput)
	} else {
		generatedStringLength = len(targetInput)
		targetObj = targets.NewStringTarget(targetInput)
	}

	if err != nil {
		fmt.Printf("Błąd: %v\n", err)
		return
	}

	// Liczba goroutines do uruchomienia
	numWorkers := *threadsFlag

	// Obliczamy łączną liczbę kombinacji do sprawdzenia.
	total := totalCombinations()

	// Wznawianie od checkpointu
	if *resumeFlag {
		if loaded := loadCheckpoint(); loaded > 0 && loaded < total {
			currentIndex, processed = loaded, loaded
			fmt.Printf("Wznowiono od indeksu %d\n", loaded)
		}
	}

	fmt.Printf("Generowanie do %d kombinacji w %d wątkach (tryb: %s)...\n", total, numWorkers, targetMode)

	startTime := time.Now()

	// Uruchamiamy zapisywanie checkpointów i monitorowanie postępu
	go checkpointSaver()
	go monitorProgress(total, startTime)

	// Uruchamianie workerów w sposób zoptymalizowany
	var wg sync.WaitGroup
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			worker(total)
		}()
	}

	// Oczekiwanie na zakończenie workerów
	wg.Wait()
	endTime := time.Now()

	fmt.Print("\n")
	fmt.Println("Znaleziono:", atomic.LoadInt32(&foundFlag) == 1, finalString)
	fmt.Printf("Czas wykonania: %v\n", endTime.Sub(startTime))
}

// --------------------------------------
// FUNKCJE POMOCNICZE
// --------------------------------------

func detectTargetMode(flagType, target string) string {
	flagType = strings.ToLower(flagType)
	if flagType != "" {
		return flagType
	}

	// Automatyczne wykrywanie na podstawie rozszerzenia pliku
	if fi, err := os.Stat(target); err == nil && !fi.IsDir() && strings.HasSuffix(strings.ToLower(target), ".zip") {
		return "zip"
	}
	return "string"
}
