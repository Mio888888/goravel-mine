package moduleadmission

import (
	"fmt"
	"sort"
	"strings"
)

func NewAdmissionLock(index RepositoryIndex, resolution Resolution) (AdmissionLock, error) {
	if index.Digest == "" || resolution.IndexDigest != index.Digest || len(resolution.Modules) == 0 {
		return AdmissionLock{}, fmt.Errorf("resolution is not bound to repository index")
	}
	modules := append([]ModuleIndexEntry(nil), resolution.Modules...)
	sort.Slice(modules, func(left, right int) bool {
		if modules[left].ID == modules[right].ID {
			return modules[left].Version < modules[right].Version
		}
		return modules[left].ID < modules[right].ID
	})
	sourcePayload := make([]string, 0, len(modules))
	for _, module := range modules {
		sourcePayload = append(sourcePayload, module.ID+"@"+module.Version+":"+module.Digest)
	}
	lock := AdmissionLock{
		SchemaVersion: "v1", IndexDigest: index.Digest, SourceDigest: sha256Digest([]byte(strings.Join(sourcePayload, "\n"))),
		DependencyGraphDigest: resolution.GraphDigest, Modules: modules,
	}
	payload, err := lock.canonicalJSON()
	if err != nil {
		return AdmissionLock{}, err
	}
	lock.Digest = sha256Digest(payload)
	return lock, nil
}

func (l AdmissionLock) JSON() ([]byte, error) {
	if strings.TrimSpace(l.Digest) == "" {
		return nil, fmt.Errorf("admission lock digest is required")
	}
	return jsonMarshal(l)
}

func (l AdmissionLock) canonicalJSON() ([]byte, error) {
	return jsonMarshal(struct {
		SchemaVersion         string             `json:"schema_version"`
		IndexDigest           string             `json:"index_digest"`
		SourceDigest          string             `json:"source_digest"`
		DependencyGraphDigest string             `json:"dependency_graph_digest"`
		Modules               []ModuleIndexEntry `json:"modules"`
	}{l.SchemaVersion, l.IndexDigest, l.SourceDigest, l.DependencyGraphDigest, l.Modules})
}

func (l AdmissionLock) ApprovalBinding(registryDigest string) string {
	return sha256Digest([]byte(strings.Join([]string{l.IndexDigest, l.SourceDigest, l.DependencyGraphDigest, registryDigest}, "\n")))
}

func ValidateAdmissionApproval(approval AdmissionApproval, lock AdmissionLock, registry StaticRegistry) error {
	if err := approval.Valid(); err != nil {
		return err
	}
	if approval.BindingDigest != lock.ApprovalBinding(registry.Digest) {
		return fmt.Errorf("module admission approval binding digest mismatch")
	}
	return nil
}

func (l AdmissionLock) VerifyGeneratedRegistry(registry StaticRegistry) error {
	if strings.TrimSpace(l.Digest) == "" || strings.TrimSpace(registry.Digest) == "" {
		return fmt.Errorf("admission lock or generated registry digest is missing")
	}
	if !strings.Contains(registry.Source, `const AdmittedRegistryLockDigest = "`+l.Digest+`"`) {
		return fmt.Errorf("generated registry is not bound to admission lock digest")
	}
	if sha256Digest([]byte(registry.Source)) != registry.Digest {
		return fmt.Errorf("generated registry digest mismatch")
	}
	return nil
}
