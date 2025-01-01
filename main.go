package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// --------------------------------------
// KONFIGURACJA / ZMIENNE GLOBALNE
// --------------------------------------

// Długość generowanych stringów.
var generatedStringLength = 8

// Szukany ciąg (musi mieć tę samą liczbę znaków, co generatedStringLength).
var targetString = "qwsgds2a"

// Flaga sygnalizująca, że już znaleźliśmy nasz ciąg.
var foundFlag int32

// Przechowuje znaleziony ciąg (jeśli uda się go znaleźć).
var finalString string

// Licznik wszystkich dotąd *wygenerowanych* stringów (czyli ile sprawdziliśmy).
var processed int64

// Globalny licznik indeksu, od którego pobierają wątki (zamiast chunków).
// Każdy wątek wykonuje: index := atomic.AddInt64(&currentIndex, 1) - 1
// i generuje string z "index".
var currentIndex int64

// characters_set_1 = []rune{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9'}
var characters_set_2 = []rune{
	'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h',
	'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p',
	'q', 'r', 's', 't', 'u', 'v', 'w', 'x',
	'y', 'z', '0', '1', '2', '3', '4', '5',
	'6', '7', '8', '9',
}

// Zbiór znaków używany do generowania
var lethers = characters_set_2

// Nazwa pliku, do którego zapisujemy postęp.
const checkpointFile = "checkpoint.txt"

// Interwał (co ile sekund) chcemy zapisywać checkpoint.
var checkpointInterval = 5 * time.Second

// --------------------------------------
// FUNKCJA MAIN
// --------------------------------------
func main() {
	// Flagi: liczba wątków i flaga -resume.
	threadsFlag := flag.Int("threads", runtime.NumCPU(), "Liczba wątków (goroutines)")
	resumeFlag := flag.Bool("resume", false, "Czy wznowić od zapisanego checkpointu (domyślnie false)")
	flag.Parse()

	numWorkers := *threadsFlag

	// Ustawiamy liczbę rdzeni (GOMAXPROCS).
	runtime.GOMAXPROCS(numWorkers)

	// Obliczamy łączną liczbę kombinacji.
	total := totalCombinations()

	// Ewentualne wczytanie postępu z pliku checkpoint.txt
	// jeśli uruchomimy z parametrem -resume
	if *resumeFlag {
		loaded := loadCheckpoint()
		if loaded > 0 && loaded < total {
			currentIndex = loaded
			processed = loaded
			fmt.Printf("Wznowiono od indeksu %d\n", loaded)
		}
	}

	fmt.Printf("Generowanie do %d kombinacji w %d wątkach...\n", total, numWorkers)

	// Start pomiaru czasu.
	startTime := time.Now()

	// Uruchamiamy wątek monitorujący postęp (co sekundę).
	var wg sync.WaitGroup

	// Uruchamiamy wątek zapisujący postępy (checkpointy).
	// Zakończy się automatycznie po znalezieniu targetu (foundFlag=1) albo po przejściu całego zakresu.
	go checkpointSaver()

	// Uruchamiamy wątek monitorujący (progress w jednej linii).
	go monitorProgress(total, startTime)

	// Uruchamiamy worker-y:
	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go func() {
			defer wg.Done()
			worker(total)
		}()
	}

	// Czekamy, aż wszystkie wątki się zakończą.
	wg.Wait()

	// Zakończ pomiar czasu
	endTime := time.Now()

	// "Wyczyszczamy" linię postępu i przechodzimy do nowej.
	// (\r - powrót karetki, \n - nowa linia)
	fmt.Print("\r\n")

	// Wyświetlamy podsumowanie
	fmt.Println("Znaleziono:", atomic.LoadInt32(&foundFlag) == 1, finalString)
	fmt.Printf("Czas wykonania: %v\n", endTime.Sub(startTime))
}

// --------------------------------------
// WORKER - GŁÓWNY BRUTE-FORCE
// --------------------------------------
func worker(total int64) {
	for {
		// Pobieramy kolejny indeks w sposób atomowy
		idx := atomic.AddInt64(&currentIndex, 1) - 1
		if idx >= total {
			// Wszystkie możliwe ciągi wygenerowane
			return
		}

		// Generujemy string
		genStr := indexToString(idx)

		// Zwiększamy licznik wygenerowanych (przetworzonych) stringów
		atomic.AddInt64(&processed, 1)

		// Czy trafiliśmy na szukany ciąg?
		if checkString(genStr) {
			atomic.StoreInt32(&foundFlag, 1)
			finalString = genStr
			return
		}

		// Jeśli inny wątek już znalazł ciąg, nie ma sensu dalej szukać
		if atomic.LoadInt32(&foundFlag) == 1 {
			return
		}
	}
}

// --------------------------------------
// CHECKPOINT SAVER
// Zapisywanie postępu (aktualnego indeksu)
// co pewien ustalony interwał czasowy
// --------------------------------------
func checkpointSaver() {
	ticker := time.NewTicker(checkpointInterval)
	defer ticker.Stop()

	for {
		<-ticker.C
		// Jeśli znaleziono target lub przebrnęliśmy przez wszystko, kończymy zapisywanie
		if atomic.LoadInt32(&foundFlag) == 1 {
			return
		}

		// Pobieramy bieżący indeks
		idx := atomic.LoadInt64(&currentIndex)
		// Zapisujemy do pliku, np. "checkpoint.txt"
		saveCheckpoint(idx)
	}
}

// Zapisywanie wartości indeksu do pliku
func saveCheckpoint(idx int64) {
	data := []byte(strconv.FormatInt(idx, 10))
	err := os.WriteFile(checkpointFile, data, 0644)
	if err != nil {
		// W realnym kodzie można logować błędy
		fmt.Printf("\nBłąd zapisu checkpointu: %v\n", err)
	}
}

// Odczytanie wartości indeksu z pliku, jeśli istnieje.
func loadCheckpoint() int64 {
	data, err := os.ReadFile(checkpointFile)
	if err != nil {
		// Plik nie istnieje lub błąd odczytu => 0
		return 0
	}

	val, err := strconv.ParseInt(string(data), 10, 64)
	if err != nil {
		return 0
	}
	return val
}

// --------------------------------------
// FUNKCJA MONITORUJĄCA POSTĘP
// (strings/s, % ukończenia, ETA) w jednej linii
// --------------------------------------
func monitorProgress(total int64, startTime time.Time) {
	var lastProcessed int64
	lastTime := time.Now()

	for {
		time.Sleep(1 * time.Second)

		// Jeśli znaleziono łańcuch, przerywamy
		if atomic.LoadInt32(&foundFlag) == 1 {
			return
		}

		now := time.Now()
		currentProcessed := atomic.LoadInt64(&processed)
		diff := currentProcessed - lastProcessed
		elapsedSec := now.Sub(lastTime).Seconds()

		// strings/s
		speed := float64(diff) / elapsedSec

		// Procent ukończenia
		progress := float64(currentProcessed) / float64(total) * 100

		// Szacowany czas do końca
		var etaSec float64
		if speed > 0 {
			etaSec = float64(total-currentProcessed) / speed
		}

		// Nadpisywanie linii (\r)
		fmt.Printf("\rPrzetworzone: %d (%.2f%%), Szybkość: ~%.0f/s, ETA: ~%.2fs",
			currentProcessed, progress, speed, etaSec)

		lastProcessed = currentProcessed
		lastTime = now

		// Gdy currentProcessed == total, też możemy przerwać
		if currentProcessed >= total {
			return
		}
	}
}

// --------------------------------------
// OBLICZENIE CAŁKOWITEJ LICZBY KOMBINACJI
// --------------------------------------
func totalCombinations() int64 {
	// len(lethers)^generatedStringLength
	var total int64 = 1
	for i := 0; i < generatedStringLength; i++ {
		total *= int64(len(lethers))
	}
	return total
}

// Zamiana liczby (index) na ciąg znaków w systemie o podstawie len(lethers).
func indexToString(index int64) string {
	base := int64(len(lethers))
	bytes := make([]rune, generatedStringLength)

	for i := generatedStringLength - 1; i >= 0; i-- {
		bytes[i] = lethers[index%base]
		index /= base
	}
	return string(bytes)
}

// Sprawdzenie, czy genStr to nasz szukany ciąg
func checkString(genStr string) bool {
	return genStr == targetString
}
