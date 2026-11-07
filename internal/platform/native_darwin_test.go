package platform

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSigningRequirementForCurrentExecutable(t *testing.T) {
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	requirement, err := SigningRequirement(path)
	if err != nil {
		t.Fatal(err)
	}
	if requirement == "" {
		t.Fatal("empty designated requirement")
	}
	if err = CheckSignature(path, requirement); err != nil {
		t.Fatal(err)
	}
	if err = CheckSelfRequirement(requirement); err != nil {
		t.Fatal(err)
	}
	if CheckSelfRequirement(`identifier "dev.nativejet.wrong"`) == nil {
		t.Fatal("accepted another application identity")
	}
	if _, err = Requirement("dev.nativejet.app", "invalid"); err == nil {
		t.Fatal("accepted malformed Team ID")
	}
}
func TestBuiltApplicationSignature(t *testing.T) {
	path, err := filepath.Abs("../../build/NativeJet.app")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); err != nil {
		t.Skip("App has not been built")
	}
	if requirement, err := SigningRequirement(path); err != nil || requirement == "" {
		t.Fatalf("built application signature: %v", err)
	}
}
