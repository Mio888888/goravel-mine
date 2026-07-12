package commands

import "testing"

func TestModuleOpenAPILintCommandContract(t *testing.T) {
	command := &ModuleOpenAPILintCommand{}
	if command.Signature() != "module:openapi:lint" {
		t.Fatalf("signature = %s", command.Signature())
	}
	flags := governanceFlags(t, command.Extend().Flags)
	if len(flags) != 2 || flags[0].Name != "all" || flags[1].Name != "bundle" {
		t.Fatalf("flags = %#v", flags)
	}
}
