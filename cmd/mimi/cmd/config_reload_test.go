//nolint:testpackage // drives the command tree through runCommand
package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/y3owk1n/mimi/internal/daemon"
	"github.com/y3owk1n/mimi/internal/ipc"
)

// hupSleeperEnv makes the copy of this test binary that startHUPSleeper
// starts ignore SIGHUP and sleep, standing in for a daemon that takes the
// signal and keeps running.
const hupSleeperEnv = "MIMI_TEST_HUP_SLEEPER"

func TestHelperHUPSleeper(t *testing.T) {
	if os.Getenv(hupSleeperEnv) != "1" {
		t.Skip("only runs as the sleeper startHUPSleeper starts")
	}

	signal.Ignore(syscall.SIGHUP)
	time.Sleep(time.Minute)
}

// startHUPSleeper starts a copy of this test binary that outlives SIGHUP, and
// returns its pid. It has this binary's name, as a running mimi has.
func startHUPSleeper(t *testing.T) int {
	t.Helper()

	cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestHelperHUPSleeper$")

	cmd.Env = append(os.Environ(), hupSleeperEnv+"=1")

	err := cmd.Start()
	if err != nil {
		t.Fatalf("starting the sleeper: %v", err)
	}

	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	// Give it the moment it needs to ignore SIGHUP before anything sends one.
	time.Sleep(200 * time.Millisecond)

	return cmd.Process.Pid
}

// reloadingDaemon answers status on socketPath with no reload until it has
// answered once, and with after from then on, the way a daemon reports the
// reload the signal between those two requests asked for.
func reloadingDaemon(t *testing.T, socketPath string, after daemon.Reload) {
	t.Helper()

	lc := net.ListenConfig{}

	listener, err := lc.Listen(context.Background(), "unix", socketPath)
	if err != nil {
		t.Fatalf("listening on fake daemon socket: %v", err)
	}

	t.Cleanup(func() { _ = listener.Close() })

	var answered atomic.Int64

	go func() {
		for {
			conn, acceptErr := listener.Accept()
			if acceptErr != nil {
				return
			}

			_, _ = bufio.NewReader(conn).ReadBytes('\n')

			status := daemon.Status{Version: Version}
			if answered.Add(1) > 1 {
				status.LastReload = &after
			}

			data, err := json.Marshal(status)
			if err == nil {
				data, err = json.Marshal(ipc.Response{OK: true, Data: data})
			}

			if err == nil {
				_, _ = conn.Write(append(data, '\n'))
			}

			_ = conn.Close()
		}
	}()
}

// TestConfigReload_ReportsHowTheReloadWent pins what mimi config reload prints
// once the daemon has reloaded: a success with the settings that still need a
// restart or a reinstall, or the reason the config was not applied, as an
// error.
func TestConfigReload_ReportsHowTheReloadWent(t *testing.T) {
	cases := []struct {
		name     string
		reload   daemon.Reload
		wantOut  []string
		wantFail string
	}{
		{
			name: "applied with settings left over",
			reload: daemon.Reload{
				OK:            true,
				RestartOnly:   []string{"settings.log_level"},
				ReinstallOnly: []string{"settings.service_path"},
			},
			wantOut: []string{
				"Configuration reloaded\n",
				"Restart the daemon to apply: settings.log_level\n",
				"Run mimi services install to apply: settings.service_path\n",
			},
		},
		{
			name:     "not applied",
			reload:   daemon.Reload{Error: "parsing config: bad toml"},
			wantFail: "config not reloaded, the daemon keeps the one it had: parsing config: bad toml",
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()
			socket := filepath.Join(shortSocketDir(t), "mimi.sock")
			pidFile := filepath.Join(dir, "mimi.pid")

			err := os.WriteFile(pidFile, []byte(strconv.Itoa(startHUPSleeper(t))), testFilePerm)
			if err != nil {
				t.Fatalf("writing pid file: %v", err)
			}

			testCase.reload.At = time.Now()
			testCase.reload.Trigger = "sighup"
			reloadingDaemon(t, socket, testCase.reload)

			configPath := filepath.Join(dir, "config.toml")
			writeConfigFile(t, configPath, fmt.Sprintf(
				"[settings]\nsocket_file = %q\npid_file = %q\n", socket, pidFile))

			out, err := runCommand(t, "--config", configPath, "config", "reload")

			if testCase.wantFail != "" {
				if err == nil || !strings.Contains(err.Error(), testCase.wantFail) {
					t.Fatalf("config reload error = %v, want %q", err, testCase.wantFail)
				}

				return
			}

			if err != nil {
				t.Fatalf("config reload: %v\n%s", err, out)
			}

			for _, want := range testCase.wantOut {
				if !strings.Contains(out, want) {
					t.Fatalf("output lacks %q:\n%s", want, out)
				}
			}
		})
	}
}
