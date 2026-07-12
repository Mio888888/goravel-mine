package moduleadmission

import (
	"fmt"
	"os"
	"path/filepath"
)

var admissionRename = os.Rename

func WriteAdmissionArtifacts(lock AdmissionLock, registry StaticRegistry, lockPath, registryPath string) error {
	lockPayload, err := lock.JSON()
	if err != nil {
		return err
	}
	if err := lock.VerifyGeneratedRegistry(registry); err != nil {
		return err
	}
	lockPath, registryPath, err = normalizedAdmissionPaths(lockPath, registryPath)
	if err != nil {
		return err
	}
	lockTemp, err := writeAdmissionTemp(lockPath, append(lockPayload, '\n'), 0600)
	if err != nil {
		return err
	}
	defer os.Remove(lockTemp)
	registryTemp, err := writeAdmissionTemp(registryPath, []byte(registry.Source), 0600)
	if err != nil {
		return err
	}
	defer os.Remove(registryTemp)
	oldLock, lockExisted, err := readAdmissionArtifact(lockPath)
	if err != nil {
		return err
	}
	oldRegistry, registryExisted, err := readAdmissionArtifact(registryPath)
	if err != nil {
		return err
	}
	if err := admissionRename(registryTemp, registryPath); err != nil {
		return fmt.Errorf("replace admission registry: %w", err)
	}
	if err := admissionRename(lockTemp, lockPath); err != nil {
		if restoreErr := restoreAdmissionArtifact(registryPath, oldRegistry, registryExisted); restoreErr != nil {
			return fmt.Errorf("replace admission lock: %w; restore admission registry: %v", err, restoreErr)
		}
		_ = restoreAdmissionArtifact(lockPath, oldLock, lockExisted)
		return fmt.Errorf("replace admission lock: %w", err)
	}
	return nil
}

func readAdmissionArtifact(path string) ([]byte, bool, error) {
	payload, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	return payload, err == nil, err
}

func restoreAdmissionArtifact(path string, payload []byte, existed bool) error {
	if !existed {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	temporary, err := writeAdmissionTemp(path, payload, 0600)
	if err != nil {
		return err
	}
	defer os.Remove(temporary)
	return os.Rename(temporary, path)
}

func normalizedAdmissionPaths(lockPath, registryPath string) (string, string, error) {
	lockPath = filepath.Clean(lockPath)
	registryPath = filepath.Clean(registryPath)
	if lockPath == "." || registryPath == "." || lockPath == registryPath {
		return "", "", fmt.Errorf("admission lock and registry paths must be distinct")
	}
	return lockPath, registryPath, nil
}

func writeAdmissionTemp(target string, payload []byte, mode os.FileMode) (string, error) {
	directory := filepath.Dir(target)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return "", err
	}
	temporary, err := os.CreateTemp(directory, ".module-admission-")
	if err != nil {
		return "", err
	}
	temporaryName := temporary.Name()
	if err := temporary.Chmod(mode); err != nil {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
		return "", err
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		_ = os.Remove(temporaryName)
		return "", err
	}
	if err := temporary.Close(); err != nil {
		_ = os.Remove(temporaryName)
		return "", err
	}
	return temporaryName, nil
}
