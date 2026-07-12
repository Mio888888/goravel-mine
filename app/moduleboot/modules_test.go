package moduleboot

import "testing"

func TestModulesIncludesCompileTimeAdmittedRegistry(t *testing.T) {
	if err := VerifyAdmittedRegistryLockDigest("sha256:b3a089cabf3d06a0534f1b0c25b466058cb7a65c44ed3fb502750f8423ce4d35"); err != nil {
		t.Fatal(err)
	}
	if len(AdmittedModules()) != 0 {
		t.Fatal("repository fixture unexpectedly includes admitted modules")
	}
	if len(Modules().IDs()) != 8 {
		t.Fatalf("built-in registry size = %d", len(Modules().IDs()))
	}
}
