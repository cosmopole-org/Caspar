package firectl

import (
	"context"
	"fmt"
	"io"
	"kasper/src/shell/utils/future"
	"os"
	"sync"
	"time"

	adapters "kasper/src/abstract/adapters/firectl"

	firecracker "github.com/firecracker-microvm/firecracker-go-sdk"
	models "github.com/firecracker-microvm/firecracker-go-sdk/client/models"
	cmap "github.com/orcaman/concurrent-map/v2"
	log "github.com/sirupsen/logrus"
)

type ChannelWriteCloser struct {
	lock   sync.Mutex
	open   bool
	buf    []byte
	C      chan string
	closed bool
}

// Write sends the string to the channel
func (w *ChannelWriteCloser) Write(p []byte) (int, error) {
	if w.closed {
		return 0, fmt.Errorf("write to closed ChannelWriteCloser")
	}
	w.lock.Lock()
	defer w.lock.Unlock()
	if w.open {
		w.C <- string(p)
	} else {
		w.buf = append(w.buf, p...)
	}
	return len(p), nil
}

// open the channel
func (w *ChannelWriteCloser) Open() {
	w.open = true
	var initial string
	func() {
		w.lock.Lock()
		defer w.lock.Unlock()
		initial = string(w.buf)
		w.buf = []byte{}
	}()
	w.C <- initial
}

// Close closes the channel
func (w *ChannelWriteCloser) Close() error {
	if !w.closed {
		close(w.C)
		w.closed = true
	}
	return nil
}

type ChannelReader struct {
	C   chan []byte
	buf []byte
}

func (cr *ChannelReader) Read(p []byte) (int, error) {
	for len(cr.buf) == 0 {
		line, ok := <-cr.C
		if !ok {
			return 0, io.EOF
		}
		cr.buf = []byte(line)
	}
	n := copy(p, cr.buf)
	cr.buf = cr.buf[n:]
	return n, nil
}

type TerminalManager struct {
	writer *ChannelWriteCloser
	reader *ChannelReader
	done   chan struct{}
	logger *log.Entry
}

type FireCtl struct {
	VMs *cmap.ConcurrentMap[string, *adapters.VM]
}

func NewFireCtl() *FireCtl {
	m := cmap.New[*adapters.VM]()
	return &FireCtl{VMs: &m}
}

func init() {
	// Configure logrus
	log.SetFormatter(&log.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
	})
	log.SetLevel(log.InfoLevel)
	log.SetOutput(os.Stdout)
}

// NewTerminalManager creates a new terminal manager
func NewTerminalManager(writer *ChannelWriteCloser, reader *ChannelReader, logger *log.Entry) *TerminalManager {
	return &TerminalManager{
		writer: writer,
		reader: reader,
		done:   make(chan struct{}),
		logger: logger.WithField("component", "terminal"),
	}
}

// Start begins reading from the terminal and notifying listeners
func (tm *TerminalManager) Start() {
	tm.logger.Info("Starting terminal output reader")
}

// Stop terminates terminal reading
func (tm *TerminalManager) Stop() {
	close(tm.done)
	tm.writer.Close()
}

// SendCommand sends a command to the VM
func (tm *TerminalManager) SendCommand(cmd string) {
	tm.reader.C <- []byte(cmd + "\n")
	log.Println("Command sent to VM")
}

func (f *FireCtl) StopVm(id string) {
	future.Async(func() {
		vm, found := f.VMs.Get(id)
		if !found {
			return
		}
		vm.SigCh <- 1
	}, false)
}

func (f *FireCtl) GetVm(id string) *adapters.VM {
	vm, found := f.VMs.Get(id)
	if found {
		return vm
	}
	return nil
}

func (f *FireCtl) RunVm(id string, terminal chan string) {
	future.Async(func() {
		socketPath := "/opt/firecracker/vms/fc." + id + ".sock"
		const (
			kernelPath = "/opt/firecracker/kernel/vmlinux"
			rootfsPath = "/opt/firecracker/rootfs/rootfs.ext4"
		)

		// Create logger
		logger := log.New()
		logger.SetFormatter(&log.JSONFormatter{})
		logger.SetLevel(log.InfoLevel)

		// Log to file and stdout
		logFile, err := os.Create("/opt/firecracker/vms/fc.log")
		if err != nil {
			logger.Fatalf("Failed to create log file: %v", err)
		}
		defer logFile.Close()

		multiWriter := io.MultiWriter(os.Stdout, logFile)
		logger.SetOutput(multiWriter)

		mainLog := logger.WithFields(log.Fields{
			"component": "main",
			"vm_id":     "firecracker-vm-" + id,
		})

		mainLog.Info("Starting Firecracker VM manager")
		ctx := context.Background()

		// Enhanced VM configuration
		cfg := firecracker.Config{
			SocketPath:      socketPath,
			KernelImagePath: kernelPath,
			KernelArgs:      "console=ttyS0 reboot=k panic=1 pci=off init=/init",
			Drives: []models.Drive{{
				DriveID:      firecracker.String("rootfs"),
				PathOnHost:   firecracker.String(rootfsPath),
				IsRootDevice: firecracker.Bool(true),
				IsReadOnly:   firecracker.Bool(false),
			}},
			MachineCfg: models.MachineConfiguration{
				VcpuCount:  firecracker.Int64(1),
				MemSizeMib: firecracker.Int64(64),
			},
		}

		// Handle signals
		sigCh := make(chan int, 1)

		writerChannel := make(chan string, 10000)
		readerChannel := make(chan []byte, 10000)

		// Create terminal manager
		termManager := NewTerminalManager(&ChannelWriteCloser{C: writerChannel, open: false}, &ChannelReader{C: readerChannel}, mainLog)
		termManager.Start()
		defer termManager.Stop()

		log.Println("created terminal manager.")

		// Configure VM command
		cmd := firecracker.VMCommandBuilder{}.
			WithBin("/usr/local/bin/firecracker").
			WithSocketPath(socketPath).
			WithStdin(termManager.reader).
			WithStdout(termManager.writer).
			WithStderr(termManager.writer).
			Build(ctx)

		log.Println("created vm commander.")

		// Create logrus logger for Firecracker SDK
		fcLogger := log.New()
		fcLogger.SetFormatter(&log.TextFormatter{DisableTimestamp: true})
		fcLogger.SetOutput(termManager.writer)
		fcLogger.SetLevel(log.WarnLevel) // Reduce SDK log noise

		log.Println("created vm logger.")

		// Create machine
		machine, err := firecracker.NewMachine(ctx, cfg,
			firecracker.WithProcessRunner(cmd),
			firecracker.WithLogger(log.NewEntry(fcLogger)),
		)
		if err != nil {
			mainLog.Fatalf("Failed to create machine: %v", err)
		}

		log.Println("created the machine.")

		f.VMs.Set(id, &adapters.VM{Machine: machine, Terminal: termManager, SigCh: sigCh})

		// Start VM in goroutine
		go func() {
			if err := machine.Start(ctx); err != nil {
				mainLog.Fatalf("Failed to start VM: %v", err)
			}
			mainLog.Info("VM started successfully")

			// Wait for VM to exit
			if err := machine.Wait(ctx); err != nil {
				mainLog.Warnf("VM wait returned error: %v", err)
			}
		}()

		go func() {
			for output := range writerChannel {
				mainLog.WithFields(log.Fields{
					"source": "vm_output",
					"length": len(output),
				}).Debug(output)
				terminal <- output
			}
		}()

		// Wait for VM to boot
		mainLog.Info("Waiting for VM to boot...")

		log.Println("step 1 of executing vm...")

		time.Sleep(5 * time.Second)

		log.Println("step 2 of executing vm...")

		// Example: Send initial commands
		termManager.SendCommand("echo 'Hello from VM terminal'")

		log.Println("step 3 of executing vm...")

		termManager.SendCommand("uname -a")

		log.Println("step 4 of executing vm...")

		mainLog.Info("VM is running. Enter commands in this terminal")
		mainLog.Info("Special commands: exit, clear, snapshot, status, loglevel [debug|info]")
		mainLog.Info("Press Ctrl+C to shutdown VM")

		log.Println("step 5 of executing vm...")

		termManager.writer.Open()

		// Wait for shutdown signal
		<-sigCh
		mainLog.Info("Shutting down VM")

		log.Println("step 6 of executing vm...")

		// Create snapshot before shutdown
		f.CreateSnapshot(ctx, machine, mainLog)

		// Stop the VM
		if err := machine.StopVMM(); err != nil {
			mainLog.Errorf("Error stopping VM: %v", err)
		} else {
			mainLog.Info("VM stopped successfully")
		}
		terminal <- ""
	}, false)
}

func (f *FireCtl) CreateSnapshot(ctx context.Context, machine *firecracker.Machine, logger *log.Entry) {
	snapshotPath := "/opt/firecracker/snapshots/snapshot.bin"

	logger.Info("Pausing VM for snapshot")
	if err := machine.PauseVM(ctx); err == nil {
		logger.Info("Creating snapshot")
		if err := machine.CreateSnapshot(ctx, snapshotPath+".mem", snapshotPath); err != nil {
			logger.Errorf("Snapshot failed: %v", err)
		} else {
			logger.Info("Snapshot created successfully")
		}

		logger.Info("Resuming VM")
		if err := machine.ResumeVM(ctx); err != nil {
			logger.Errorf("Failed to resume VM: %v", err)
		}
	} else {
		logger.Errorf("Failed to pause VM for snapshot: %v", err)
	}
}
