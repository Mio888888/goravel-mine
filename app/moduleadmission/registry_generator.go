package moduleadmission

import (
	"fmt"
	"go/format"
	"strings"
)

func GenerateStaticRegistry(lock AdmissionLock) (StaticRegistry, error) {
	if strings.TrimSpace(lock.Digest) == "" || len(lock.Modules) == 0 {
		return StaticRegistry{}, fmt.Errorf("admission lock is incomplete")
	}
	imports := make([]string, 0, len(lock.Modules))
	for _, module := range lock.Modules {
		path := strings.TrimSpace(module.GoImportPath)
		if path == "" {
			return StaticRegistry{}, fmt.Errorf("module %s has no Go import path", module.ID)
		}
		imports = append(imports, fmt.Sprintf("\t%q", path))
	}
	source := "package moduleboot\n\nimport (\n\t\"goravel/app/modules\"\n" + strings.Join(imports, "\n") + "\n)\n\n"
	source += "const AdmittedRegistryLockDigest = \"" + lock.Digest + "\"\n\n"
	source += "func VerifyAdmittedRegistryLockDigest(expected string) error {\n\tif expected != AdmittedRegistryLockDigest {\n\t\treturn errAdmittedRegistryLockDigestMismatch\n\t}\n\treturn nil\n}\n\n"
	source += "func AdmittedModules() []modules.Module {\n\treturn []modules.Module{\n"
	for _, module := range lock.Modules {
		alias := moduleImportAlias(module.GoImportPath)
		source += "\t\t" + alias + ".New(),\n"
	}
	source += "\t}\n}\n"
	formatted, err := format.Source([]byte(source))
	if err != nil {
		return StaticRegistry{}, err
	}
	return StaticRegistry{Source: string(formatted), Digest: sha256Digest(formatted)}, nil
}

func moduleImportAlias(path string) string {
	parts := strings.Split(strings.TrimSuffix(path, "/"), "/")
	return strings.NewReplacer("-", "", "_", "").Replace(parts[len(parts)-1])
}
