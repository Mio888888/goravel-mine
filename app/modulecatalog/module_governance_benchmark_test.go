package modulecatalog_test

import (
	"context"
	"testing"

	"goravel/app/moduleboot"
	"goravel/app/modulecatalog"
)

var (
	manifestSink      modulecatalog.Manifest
	compatibilitySink modulecatalog.CompatibilityMatrix
	planSink          []modulecatalog.LifecyclePlanItem
	dryRunSink        modulecatalog.LifecycleResult
)

func BenchmarkModuleGovernanceManifest(b *testing.B) {
	service := modulecatalog.NewService(moduleboot.Modules())

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		manifestSink = service.Manifest()
	}
}

func BenchmarkModuleGovernanceCompatibility(b *testing.B) {
	service := modulecatalog.NewService(moduleboot.Modules())

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		compatibilitySink = service.CompatibilityMatrix("1.17.2")
	}
}

func BenchmarkModuleGovernancePlan(b *testing.B) {
	service := modulecatalog.NewService(moduleboot.Modules())

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		plan, err := service.LifecyclePlan(modulecatalog.LifecycleActionUpgrade)
		if err != nil {
			b.Fatal(err)
		}
		planSink = plan
	}
}

func BenchmarkModuleGovernanceDryRun(b *testing.B) {
	ctx := context.Background()
	service := modulecatalog.NewLifecycleService(moduleboot.Modules())
	opts := modulecatalog.LifecycleOptions{
		ModuleID: "platform-rbac",
		Execute:  false,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		result, err := service.Execute(ctx, modulecatalog.LifecycleActionUpgrade, opts)
		if err != nil {
			b.Fatal(err)
		}
		dryRunSink = result
	}
}
