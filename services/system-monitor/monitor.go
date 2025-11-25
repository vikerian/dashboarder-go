package main

import (
	"log/slog"
	"strings"

	// Knihovna gopsutil pro čtení systémových statistik (CPU, RAM, Disk, Procesy).
	// Funguje multiplatformně, ale my se soustředíme na Linux (Docker/RPi).
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
)

// SystemStats je datová struktura (DTO), která drží jeden "snímek" stavu systému.
// Slouží k přenosu dat z měřící funkce do funkce, která data odesílá přes MQTT.
type SystemStats struct {
	// CPULoad: Průměrné vytížení procesoru v procentech (0-100).
	CPULoad float64

	// RAM (Operační paměť)
	// Pro vykreslení koláčového grafu potřebujeme znát "Kolik je obsazeno" a "Kolik je celkem".
	RamUsedMB  float64 // Reálně využitá paměť aplikacemi (bez diskové cache)
	RamTotalMB float64 // Celková fyzická paměť instalovaná v zařízení

	// AppRamUsedMB: Součet RAM, kterou využívají pouze naše IoT služby.
	// Pomáhá nám zjistit, zda Docker kontejnery "nežerou" moc paměti.
	AppRamUsedMB float64

	// Disk (Úložiště)
	DiskUsedGB  float64 // Obsazené místo na disku
	DiskTotalGB float64 // Celková kapacita disku
}

// CollectStats je hlavní funkce pro sběr dat.
// Vrací pointer na SystemStats (aby se nekopírovala struktura) nebo chybu.
func CollectStats(logger *slog.Logger) (*SystemStats, error) {
	// Inicializace prázdné struktury
	stats := &SystemStats{}

	// =========================================================================
	// 1. MĚŘENÍ CPU (Zátěž procesoru)
	// =========================================================================
	// cpu.Percent vypočítá vytížení CPU za daný časový úsek.
	// Argument 1 (interval): 1000ms. Funkce zde na 1 sekundu "uspí" vlákno,
	// změří rozdíl v čítačích CPU na začátku a na konci, a vypočítá procento.
	// Argument 2 (percpu): false. Chceme průměr přes všechna jádra dohromady.
	percentages, err := cpu.Percent(1000, false)

	// Kontrola chyby a délky pole (aby nám program nespadl na index out of range)
	if err == nil && len(percentages) > 0 {
		stats.CPULoad = percentages[0]
	} else {
		// Logujeme chybu, ale pokračujeme dál. Nechceme, aby chyba CPU zastavila měření RAM.
		logger.Error("Chyba při čtení CPU statistik", "error", err)
	}

	// =========================================================================
	// 2. MĚŘENÍ RAM (S opravou pro Linux Cache)
	// =========================================================================
	// Načteme info o virtuální paměti (odpovídá souboru /proc/meminfo).
	vMem, err := mem.VirtualMemory()
	if err == nil {
		// DŮLEŽITÁ LEKCE O LINUXU:
		// Linux se řídí heslem "Unused RAM is wasted RAM". Volnou paměť používá
		// pro cachování souborů z disku (Buffers/Cache).
		//
		// Pokud bychom použili 'vMem.Used', zahrnovalo by to i tuto cache.
		// Na RPi by to vypadalo, že RAM je plná (např. 98%), i když aplikace berou jen 20%.
		//
		// Správný vzorec pro "Uživatelsky obsazenou RAM" je: Total - Available.
		// 'Available' je paměť, kterou může OS okamžitě uvolnit pro aplikace, když si o ni řeknou.
		realUsedBytes := vMem.Total - vMem.Available

		// Převod na Megabajty (1 MB = 1024 * 1024 B)
		stats.RamUsedMB = float64(realUsedBytes) / 1024.0 / 1024.0
		stats.RamTotalMB = float64(vMem.Total) / 1024.0 / 1024.0
	} else {
		logger.Error("Chyba při čtení RAM statistik", "error", err)
	}

	// =========================================================================
	// 3. MĚŘENÍ SPECIFICKÝCH PROCESŮ (App RAM)
	// =========================================================================
	// Chceme vědět, kolik RAM "žere" náš IoT Stack.
	// Definujeme klíčová slova, která hledáme v názvech procesů.
	targetApps := []string{
		"ingestor",      // Služba pro sběr dat
		"persister",     // Služba pro ukládání
		"home-api",      // Backend API
		"web-dashboard", // Frontend
		"mosquitto",     // MQTT Broker
		"postgres",      // Databáze
	}

	// Získáme seznam všech běžících procesů v OS.
	// (Díky 'pid: host' v Docker Compose vidíme i procesy mimo náš kontejner)
	procs, _ := process.Processes()
	var appMemSum uint64 = 0

	for _, p := range procs {
		// Získáme jméno procesu
		name, err := p.Name()
		if err != nil {
			// Proces mohl skončit během iterace, ignorujeme a jdeme dál.
			continue
		}

		// Projdeme naše klíčová slova
		for _, target := range targetApps {
			if strings.Contains(name, target) {
				// Pokud název odpovídá, zjistíme detaily paměti.
				// Používáme RSS (Resident Set Size) - to je skutečná fyzická RAM,
				// kterou proces blokuje (bez swapu a sdílených knihoven).
				memInfo, err := p.MemoryInfo()
				if err == nil {
					appMemSum += memInfo.RSS
				}
				// Našli jsme shodu, nemusíme zkoušet další klíčová slova pro tento proces.
				break
			}
		}
	}
	// Převod na MB
	stats.AppRamUsedMB = float64(appMemSum) / 1024.0 / 1024.0

	// =========================================================================
	// 4. MĚŘENÍ DISKU
	// =========================================================================
	// Měříme využití kořenového oddílu "/".
	// Uvnitř Dockeru to může ukazovat statistiky OverlayFS, ale gopsutil
	// se snaží vrátit statistiky podkladového filesystému hostitele.
	dStat, err := disk.Usage("/")
	if err == nil {
		// Převod na Gigabajty (1 GB = 1024^3 B)
		stats.DiskUsedGB = float64(dStat.Used) / 1024.0 / 1024.0 / 1024.0
		stats.DiskTotalGB = float64(dStat.Total) / 1024.0 / 1024.0 / 1024.0
	} else {
		logger.Error("Chyba při čtení statistik disku", "error", err)
	}

	return stats, nil
}
