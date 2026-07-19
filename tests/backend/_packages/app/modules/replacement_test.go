package modules

import (
	"strings"
	"testing"
	"time"
)

func TestDeprecatedPackageRequiresCompleteReplacementPlan(t *testing.T) {
	pkg := Package{
		ImportPath: "goravel/app/modules/legacy", RegistryKey: "legacy", Version: "1.0.0",
		Owner: "platform-team", ReleaseTrack: "deprecated", Compatibility: []string{">=1.17.0 <2.0.0"},
		Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Signature: "cosign:legacy.sig",
		Deprecated: true, ReplacedBy: "modern",
	}
	errs := validatePackageWithReplacement("legacy", "1.0.0", pkg, ReplacementPlan{}, false, time.Now())
	if !containsReplacementError(errs, "replacement plan") {
		t.Fatalf("validatePackage() errors = %v", errs)
	}
}

func TestReplacementPlanRequiresAllPhasesAndCommandHashes(t *testing.T) {
	plan := ReplacementPlan{
		FromModule: "legacy", ToModule: "modern", AnnouncedAt: time.Now().Add(-24 * time.Hour),
		EndOfSupport: time.Now().Add(24 * time.Hour), RemovalVersion: "2.0.0",
		DataMigration: "migrate", Validation: "module:manifest:check", Cutover: "migrate", Rollback: "migrate:rollback",
		Phases: []ReplacementPhase{ReplacementPhasePrepare, ReplacementPhaseDualRun, ReplacementPhaseCutover, ReplacementPhaseRollbackWindow, ReplacementPhaseRetired},
	}
	if err := plan.Validate("legacy", "modern", time.Now()); err == nil || !strings.Contains(err.Error(), "command policy hash") {
		t.Fatalf("plan.Validate() error = %v", err)
	}
	plan.ConfigMigration = "migrate"
	plan.PermissionMapping = "module:manifest:check"
	plan.CommandPolicyHashes = plan.CommandHashes()
	if err := plan.Validate("legacy", "modern", time.Now()); err != nil {
		t.Fatalf("plan.Validate() error = %v", err)
	}
}

func TestReplacementRemovalGateHonorsEndOfSupportAndEmergencyApproval(t *testing.T) {
	plan := completeReplacementPlan(time.Now().Add(48 * time.Hour))
	if err := plan.CanRemove(time.Now(), false); err == nil || !strings.Contains(err.Error(), "end of support") {
		t.Fatalf("CanRemove() error = %v", err)
	}
	if err := plan.CanRemove(time.Now(), true); err != nil {
		t.Fatalf("CanRemove(emergency) error = %v", err)
	}
}

func containsReplacementError(errs []error, contains string) bool {
	for _, err := range errs {
		if strings.Contains(err.Error(), contains) {
			return true
		}
	}
	return false
}

func completeReplacementPlan(endOfSupport time.Time) ReplacementPlan {
	plan := ReplacementPlan{
		FromModule: "legacy", ToModule: "modern", AnnouncedAt: endOfSupport.Add(-48 * time.Hour),
		EndOfSupport: endOfSupport, RemovalVersion: "2.0.0", DataMigration: "migrate",
		ConfigMigration: "migrate", PermissionMapping: "module:manifest:check", Validation: "module:manifest:check",
		Cutover: "migrate", Rollback: "migrate:rollback",
		Phases: []ReplacementPhase{ReplacementPhasePrepare, ReplacementPhaseDualRun, ReplacementPhaseCutover, ReplacementPhaseRollbackWindow, ReplacementPhaseRetired},
	}
	plan.CommandPolicyHashes = plan.CommandHashes()
	return plan
}
