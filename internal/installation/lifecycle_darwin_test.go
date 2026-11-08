package installation

import (
	"errors"
	"testing"
	"time"
)

func TestUpdateWaitsForUnloadAndRetriesTransientBootstrap(t *testing.T) {
	loaded, unloading := true, false
	polls, attempts := 0, 0
	run := func(args ...string) error {
		switch args[0] {
		case "print":
			if unloading {
				polls++
				if polls >= 3 {
					loaded = false
					unloading = false
				}
			}
			if !loaded {
				return errors.New("not found")
			}
		case "bootout":
			unloading = true
		case "bootstrap":
			if loaded || unloading {
				t.Fatal("bootstrap ran before old service was removed")
			}
			attempts++
			if attempts == 1 {
				return errors.New("transient bootstrap 5")
			}
			loaded = true
		}
		return nil
	}
	if err := stopService(run, time.Second); err != nil {
		t.Fatal(err)
	}
	if polls < 3 || loaded {
		t.Fatal("did not wait for complete unload")
	}
	if err := bootstrapService(run); err != nil {
		t.Fatal(err)
	}
	if !loaded || attempts != 2 {
		t.Fatal("transient launchd error was not retried")
	}
}
func TestUpdateDoesNotHidePersistentBootstrapFailure(t *testing.T) {
	attempts := 0
	failure := errors.New("invalid launch daemon")
	run := func(args ...string) error {
		if args[0] == "bootstrap" {
			attempts++
			return failure
		}
		return errors.New("not found")
	}
	if err := bootstrapService(run); !errors.Is(err, failure) || attempts != 4 {
		t.Fatal("persistent error was hidden")
	}
}
