//go:build linux

package kvm

import (
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/rs/zerolog"
)

type nativeOutput struct {
	mu     *sync.Mutex
	logger *zerolog.Event
}

func (w *nativeOutput) Write(p []byte) (n int, err error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.logger.Msg(string(p))
	return len(p), nil
}

func startVideoBinary(binaryPath string) (*exec.Cmd, error) {
	// Run the binary inthe background
	cmd := exec.Command(binaryPath)
	cmd.Env = append(os.Environ(), "LD_LIBRARY_PATH=/oem/usr/lib:/oem/lib:/usr/lib:/lib:"+os.Getenv("LD_LIBRARY_PATH"))

	vidoeOutputLock := sync.Mutex{}
	videoStdout := &nativeOutput{
		mu:     &vidoeOutputLock,
		logger: videoLogger.Info().Str("pipe", "stdout"),
	}
	videoStderr := &nativeOutput{
		mu:     &vidoeOutputLock,
		logger: videoLogger.Info().Str("pipe", "stderr"),
	}

	// Redirect stdout and stderr to the current process
	cmd.Stdout = videoStdout
	cmd.Stderr = videoStderr

	// Set the process group ID so we can kill the process and its children when this process exits
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start binary: %w", err)
	}

	return cmd, nil
}

func startAudioBinary(binaryPath string) (*exec.Cmd, error) {
	// Run the binary inthe background
	cmd := exec.Command(binaryPath)

	audioOutputLock := sync.Mutex{}
	audioStdout := &nativeOutput{
		mu:     &audioOutputLock,
		logger: audioLogger.Info().Str("pipe", "stdout"),
	}
	audioStderr := &nativeOutput{
		mu:     &audioOutputLock,
		logger: audioLogger.Info().Str("pipe", "stderr"),
	}

	// Redirect stdout and stderr to the current process
	cmd.Stdout = audioStdout
	cmd.Stderr = audioStderr

	// Set the process group ID so we can kill the process and its children when this process exits
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start binary: %w", err)
	}

	return cmd, nil
}

func startVpnBinary(binaryPath string) (*exec.Cmd, error) {
	// Run the binary inthe background
	cmd := exec.Command(binaryPath)

	vpnOutputLock := sync.Mutex{}
	vpnStdout := &nativeOutput{
		mu:     &vpnOutputLock,
		logger: audioLogger.Info().Str("pipe", "stdout"),
	}
	vpnStderr := &nativeOutput{
		mu:     &vpnOutputLock,
		logger: audioLogger.Info().Str("pipe", "stderr"),
	}

	// Redirect stdout and stderr to the current process
	cmd.Stdout = vpnStdout
	cmd.Stderr = vpnStderr

	// Set the process group ID so we can kill the process and its children when this process exits
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start binary: %w", err)
	}

	return cmd, nil
}

func startDisplayBinary(binaryPath string) (*exec.Cmd, error) {
	// Run the binary inthe background
	cmd := exec.Command(binaryPath)
	cmd.Env = append(os.Environ(), "LD_LIBRARY_PATH=/oem/usr/lib:/oem/lib:/usr/lib:/lib:"+os.Getenv("LD_LIBRARY_PATH"))

	displayOutputLock := sync.Mutex{}
	displayStdout := &nativeOutput{
		mu:     &displayOutputLock,
		logger: displayLogger.Info().Str("pipe", "stdout"),
	}
	displayStderr := &nativeOutput{
		mu:     &displayOutputLock,
		logger: displayLogger.Info().Str("pipe", "stderr"),
	}

	//// Redirect stdout and stderr to the current process
	cmd.Stdout = displayStdout
	cmd.Stderr = displayStderr

	// Set the process group ID so we can kill the process and its children when this process exits
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid:   true,
		Pdeathsig: syscall.SIGKILL,
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start binary: %w", err)
	}

	return cmd, nil
}
