package moduleboot

import "goravel/app/modules"

const AdmittedRegistryLockDigest = "sha256:b3a089cabf3d06a0534f1b0c25b466058cb7a65c44ed3fb502750f8423ce4d35"

func VerifyAdmittedRegistryLockDigest(expected string) error {
	if expected != AdmittedRegistryLockDigest {
		return errAdmittedRegistryLockDigestMismatch
	}
	return nil
}

func AdmittedModules() []modules.Module {
	return nil
}
