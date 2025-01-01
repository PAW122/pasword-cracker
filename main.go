package main

import (
	"flag"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

var (
	// Domyślna długość generowanego stringa:
	generatedStringLength = 6

	// Przykładowy target — 8 znaków
	targetString = "qwe123"

	// Flaga sygnalizująca, że już znaleźliśmy nasz ciąg
	foundFlag int32

	// Przechowuje znaleziony ciąg (jeśli uda się go znaleźć)
	finalString string

	// Licznik wszystkich dotąd wygenerowanych stringów
	processed int64

	//characters_set_1 = []rune{'0', '1', '2', '3', '4', '5', '6', '7', '8', '9'}
	characters_set_2 = []rune{
		'a', 'b', 'c', 'd', 'e', 'f', 'g', 'h',
		'i', 'j', 'k', 'l', 'm', 'n', 'o', 'p',
		'q', 'r', 's', 't', 'u', 'v', 'w', 'x',
		'y', 'z', '0', '1', '2', '3', '4', '5',
		'6', '7', '8', '9',
	}
	// Zbiór znaków używany do generowania
	lethers = characters_set_2
)

func main() {
	// 1. Odczyt argumentu z linii poleceń – liczba wątków
	//    Jeśli nie podamy -threads, użyjemy liczby CPU w systemie.
	threadsFlag := flag.Int("threads", runtime.NumCPU(), "Liczba wątków (goroutines)")
	flag.Parse()

	numWorkers := *threadsFlag
	// Ograniczamy też GOMAXPROCS, żeby użyć właśnie tylu rdzeni
	runtime.GOMAXPROCS(numWorkers)

	// Całkowita liczba możliwych kombinacji
	total := totalCombinations()
	fmt.Printf("Generowanie do %d kombinacji w %d wątkach...\n", total, numWorkers)

	// Podział zakresu na wątki
	chunkSize := total / int64(numWorkers)
	if total%int64(numWorkers) != 0 {
		chunkSize++
	}

	// Zaczynamy pomiar czasu
	startTime := time.Now()

	var wg sync.WaitGroup
	wg.Add(numWorkers)

	// 2. Osobna goroutine do monitorowania postępu
	go monitorProgress(total, startTime)

	// 3. Uruchamiamy wątki (worker-y)
	for i := 0; i < numWorkers; i++ {
		start := int64(i) * chunkSize
		end := start + chunkSize
		if end > total {
			end = total
		}

		go func(s, e int64) {
			defer wg.Done()
			worker(s, e)
		}(start, end)
	}

	// Czekamy, aż wszystkie goroutines się zakończą
	wg.Wait()

	// Zakończ pomiar czasu
	endTime := time.Now()

	// Wyświetlamy podsumowanie
	// Najpierw wyczyść (lub przejdź do nowej linii), by "nie nadpisać" linijki statusu
	fmt.Print("\r")
	fmt.Println("\n\n\n\nZnaleziono:", atomic.LoadInt32(&foundFlag) == 1, finalString)
	fmt.Printf("Czas wykonania: %v\n", endTime.Sub(startTime))
}

// worker sprawdza ciągi w zakresie od start do end-1
func worker(start, end int64) {
	for index := start; index < end; index++ {
		// Jeśli inny wątek już znalazł ciąg, kończymy pętlę
		if atomic.LoadInt32(&foundFlag) == 1 {
			return
		}

		genStr := indexToString(index)

		// Zwiększamy globalny licznik wygenerowanych stringów
		atomic.AddInt64(&processed, 1)

		if checkString(genStr) {
			// Ustawiamy flagę i zapamiętujemy ciąg
			atomic.StoreInt32(&foundFlag, 1)
			finalString = genStr
			return
		}
	}
}

// monitorProgress - osobna goroutine, która co sekundę aktualizuje postęp na 1 linii
func monitorProgress(total int64, startTime time.Time) {
	var lastProcessed int64
	lastTime := time.Now()

	for {
		// Sprawdzamy co 1 sekundę
		time.Sleep(1 * time.Second)

		// Jeśli już znaleziono target (foundFlag == 1), kończymy monitorowanie
		if atomic.LoadInt32(&foundFlag) == 1 {
			return
		}

		// Aktualne statystyki
		now := time.Now()
		currentProcessed := atomic.LoadInt64(&processed)

		diff := currentProcessed - lastProcessed
		elapsedSec := now.Sub(lastTime).Seconds()

		// Liczba stringów/s od ostatniego pomiaru
		speed := float64(diff) / elapsedSec

		// Ogólny procent ukończenia
		progress := float64(currentProcessed) / float64(total) * 100

		// Szacowany czas do końca (ETA)
		var etaSec float64
		if speed > 0 {
			etaSec = float64(total-currentProcessed) / speed
		}

		// Wypisanie w jednej linii za pomocą \r:
		// - \r cofa kursor na początek linii,
		// - nie dodajemy \n (nowej linii), żeby nadpisać bieżącą linijkę.
		//
		// Dla pewności można użyć sekwencji ANSI "\033[K" (kasowanie do końca linii)
		// np. fmt.Printf("\r\033[KPrzetworzone: ...")
		// ale samo \r często wystarczy.
		fmt.Printf("\rPrzetworzone: %d (%.2f%%), Szybkość: ~%.0f/s, ETA: ~%.2fs",
			currentProcessed, progress, speed, etaSec)

		// Uaktualniamy stan do następnego odczytu
		lastProcessed = currentProcessed
		lastTime = now
	}
}

// totalCombinations oblicza liczbę możliwych kombinacji
func totalCombinations() int64 {
	// len(lethers)^generatedStringLength
	var total int64 = 1
	for i := 0; i < generatedStringLength; i++ {
		total *= int64(len(lethers))
	}
	return total
}

// indexToString - zamienia liczbę (index) na ciąg znaków
// w systemie o podstawie len(lethers).
func indexToString(index int64) string {
	base := int64(len(lethers))
	bytes := make([]rune, generatedStringLength)

	for i := generatedStringLength - 1; i >= 0; i-- {
		bytes[i] = lethers[index%base]
		index /= base
	}
	return string(bytes)
}

// checkString sprawdza, czy genStr to nasz szukany ciąg
func checkString(genStr string) bool {
	return genStr == targetString
}
