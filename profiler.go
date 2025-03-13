package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	gasFee           = 10000000
	gasWanted        = 800000
	csvFile          = "pc_profiler.csv"
	MaxPackageLength = 20
	BalanceQuery     = "gnokey query bank/balances/g1jg8mtutu9khhfwc4nxmuhcpftf0pajdhfvsqf5"
	DefaultChainId   = "dev"
)

type ExecutionLog struct {
	Timestamp    time.Time
	ResponseTime time.Duration
}

type CommandLineArgs struct {
	MaxThreads   int
	MaxQPS       int
	Mode         string
	PackageName  string
	FunctionName string
	Remote       string
	KeyName      string
	PkgDir       string
	ChainID      string
}

func validateArgs(args CommandLineArgs) {

	// Validate mode-based argument requirements
	if args.Mode == "addpkg" && args.FunctionName != "" {
		fmt.Println("Error: function argument should not be provided in addpkg mode")
		os.Exit(1)
	}
	if args.Mode == "call" && args.PackageName == "" {
		fmt.Println("Error: package argument must be specified in call mode.")
		os.Exit(1)
	}

	if args.Mode == "balanceQuery" {
		if args.PackageName != "" {
			fmt.Println("Error: Cannot specify packageName in balanceQuery mode.")
			os.Exit(1)
		}
		if args.FunctionName != "" {
			fmt.Println("Error: Cannot specify function in balanceQuery mode.")
			os.Exit(1)
		}
		if args.PkgDir != "." {
			fmt.Println("Error: Cannot specify pkgDir in balanceQuery mode.")
			os.Exit(1)
		}
	}

	//if args.MaxThreads > 1 {
	//	fmt.Println("Error: More than 1 thread not yet supported (TODO).")
	//	os.Exit(1)
	//}

	if args.Mode == "qrender" {
		if args.PackageName == "" {
			fmt.Println("Error: package must be specified in qrender mode.")
			os.Exit(1)
		}

		if args.ChainID != DefaultChainId {
			// TODO: Verify this is true of gnokey
			fmt.Println("Error: Chain ID cannot be specified in qrender mode.")
			os.Exit(1)
		}
	}

}

func main() {
	// Command-line argument parsing
	maxThreads := flag.Int("maxThreads", 1, "Max number of simultaneous threads")
	maxQPS := flag.Int("maxQueriesPerSec", 1, "Max queries per second per thread")
	mode := flag.String("mode", "call", "Mode: addpkg, addpkg+call, call, balanceQuery, or qrender")
	packageName := flag.String("package", "", "Package name (required for addpkg mode or qrender mode)")
	functionName := flag.String("function", "", "Function name (required for call modes)")
	remote := flag.String("remote", "localhost:26657", "Remote endpoint")
	keyName := flag.String("keyname", "Dev", "Key name")
	pkgDir := flag.String("pkgdir", ".", "Package directory")
	chainID := flag.String("chainid", DefaultChainId, "Chain ID")

	flag.Parse()

	validateArgs(CommandLineArgs{
		MaxThreads:   *maxThreads,
		MaxQPS:       *maxQPS,
		Mode:         *mode,
		PackageName:  *packageName,
		FunctionName: *functionName,
		Remote:       *remote,
		KeyName:      *keyName,
		PkgDir:       *pkgDir,
		ChainID:      *chainID,
	})

	// Check if there is input from stdin
	fi, err := os.Stdin.Stat()
	if err != nil {
		fmt.Println("Error checking stdin:", err)
		os.Exit(1)
	}

	// If stdin has data (it's not from a terminal), read the password
	var password string
	if (fi.Mode() & os.ModeCharDevice) == 0 {
		stdinScanner := bufio.NewScanner(os.Stdin)
		if stdinScanner.Scan() {
			password = stdinScanner.Text()
		}
	} else {
		password = "" // Default to empty string if no input is piped
	}

	// Channel to manage worker pool
	sem := make(chan struct{}, *maxThreads)
	var wg sync.WaitGroup

	// Track execution times
	var logs []ExecutionLog
	var logMutex sync.Mutex

	// Handle graceful shutdown
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-signalChan
		fmt.Println("\nStopping workers and saving logs...")
		saveLogs(logs)
		os.Exit(0)
	}()

	fmt.Println("INFO: About to start worker threads...")

	// Start worker threads
	for {
		sem <- struct{}{}
		wg.Add(1)
		go func() {
			defer func() {
				<-sem
				wg.Done()
			}()
			executeTask(*mode, *packageName, *functionName, *remote, *keyName, *pkgDir, *chainID, *maxQPS, password, &logs, &logMutex)
		}()
	}
}

func executeTask(mode, packageName, functionName, remote, keyName, pkgDir, chainID string, maxQPS int, password string, logs *[]ExecutionLog, logMutex *sync.Mutex) {
	queryCount := 0
	lastQueryTime := time.Now()

	firstLoop := true

	for {
		if time.Since(lastQueryTime) >= time.Second {
			queryCount = 0
			lastQueryTime = time.Now()
		}
		if queryCount >= maxQPS {
			time.Sleep(time.Until(lastQueryTime.Add(time.Second)))
			continue
		}
		queryCount++

		// Must generate 2 commands for addpkg+call as both may require passing a gnokey password
		// via stdin
		firstMode := mode
		if firstMode == "addpkg+call" {
			firstMode = "addpkg"

			//need the same packageName for both addpkg and call

			if packageName == "" {
				packageName = randomString(MaxPackageLength)
			}
		}

		cmdStr := generateCommand(firstMode, packageName, functionName, remote, keyName, pkgDir, chainID)

		if firstLoop {
			fmt.Println("INFO: Executing", cmdStr)
		}

		start := time.Now()
		//out, err := executeCommand(cmdStr, password)
		_, err := executeCommand(cmdStr, password)
		if err != nil {
			fmt.Println("WARNING: Errors executing command: ", err)
		}

		if mode == "addpkg+call" {
			cmdStr2 := generateCommand("call", packageName, functionName, remote, keyName, pkgDir, chainID)
			executeCommand(cmdStr2, password)

			//Must reset packageName so it's random for the next invocation
			packageName = ""

			if firstLoop {
				fmt.Println("INFO: Executing", cmdStr2)
			}
		}
		duration := time.Since(start)
		fmt.Println("Completed gnokey command in", duration.Seconds(), "seconds.")

		firstLoop = false

		logMutex.Lock()
		*logs = append(*logs, ExecutionLog{Timestamp: time.Now(), ResponseTime: duration})
		logMutex.Unlock()
	}
}

func generateCommand(mode, packageName, functionName, remote, keyName, pkgDir, chainID string) string {
	if packageName == "" {
		packageName = randomString(MaxPackageLength)
	}
	if functionName == "" {
		functionName = "Main"
	}

	switch mode {
	case "addpkg":
		return fmt.Sprintf(
			"gnokey maketx addpkg --pkgpath 'gno.land/r/%s' --pkgdir %s "+
				"--gas-fee %dugnot --gas-wanted %d --broadcast "+
				"--chainid %s --remote %s --insecure-password-stdin=true %s",
			packageName, pkgDir, gasFee, gasWanted, chainID, remote, keyName,
		)
	case "addpkg+call":
		panic("Programming error: addpkg+call should be 2 separate calls to generateCommand.")
	case "call":
		return fmt.Sprintf(
			"gnokey maketx call --pkgpath 'gno.land/r/%s' --func %s "+
				"--gas-fee %dugnot --gas-wanted %d --broadcast "+
				"--chainid %s --remote %s --insecure-password-stdin=true %s",
			packageName, functionName, gasFee, gasWanted, chainID, remote, keyName,
		)
	case "balanceQuery":
		return BalanceQuery
	case "qrender":
		//TODO: support specifying args for qrender instead of only being able to call with ""
		return fmt.Sprintf("gnokey query vm/qrender --data '%s:' --remote %s", packageName, remote)
	}
	panic("Invalid mode")
}

func executeCommand(command, password string) (string, error) {
	cmd := exec.Command("bash", "-c", command)

	// Ensure password is passed correctly via stdin
	if password != "" {
		cmd.Stdin = strings.NewReader(password + "\n") // Ensures newline termination
	} else {
		cmd.Stdin = strings.NewReader("\n") // Ensures stdin isn't empty
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	err := cmd.Run()

	// Print stderr for debugging or noticing when something has crashed
	if err != nil {
		fmt.Println("Command error:", err)
		fmt.Println("stderr:", stderr.String())
	}

	return out.String(), err
}

func saveLogs(logs []ExecutionLog) {
	file, err := os.Create(csvFile)
	if err != nil {
		fmt.Println("Failed to create CSV file:", err)
		return
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()
	writer.Write([]string{"Timestamp", "ResponseTime"})
	for _, log := range logs {
		writer.Write([]string{log.Timestamp.Format(time.RFC3339), fmt.Sprintf("%f", log.ResponseTime.Seconds())})
		writer.Flush()
	}
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}
