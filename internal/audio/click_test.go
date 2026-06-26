package audio

import "testing"

func TestSudoInvokingUserRuntimeDir_NonRoot(t *testing.T) {
	t.Parallel()

	getenv := func(string) string { return "1000" }
	if _, ok := sudoInvokingUserRuntimeDir(getenv, 1000); ok {
		t.Fatal("expected non-root to return no runtime dir")
	}
}

func TestSudoInvokingUserRuntimeDir_MissingUID(t *testing.T) {
	t.Parallel()

	getenv := func(string) string { return "" }
	if _, ok := sudoInvokingUserRuntimeDir(getenv, 0); ok {
		t.Fatal("expected missing SUDO_UID to return no runtime dir")
	}
}

func TestSudoInvokingUserRuntimeDir_InvalidUID(t *testing.T) {
	t.Parallel()

	getenv := func(string) string { return "abc" }
	if _, ok := sudoInvokingUserRuntimeDir(getenv, 0); ok {
		t.Fatal("expected invalid SUDO_UID to return no runtime dir")
	}
}

func TestSudoInvokingUserRuntimeDir_ValidUID(t *testing.T) {
	t.Parallel()

	getenv := func(key string) string {
		if key == "SUDO_UID" {
			return "1000"
		}
		return ""
	}

	runtimeDir, ok := sudoInvokingUserRuntimeDir(getenv, 0)
	if !ok {
		t.Fatal("expected valid sudo context to return runtime dir")
	}
	if runtimeDir != "/run/user/1000" {
		t.Fatalf("expected /run/user/1000, got %q", runtimeDir)
	}
}
