package facades

import "github.com/goravel/framework/contracts/telemetry"

type telemetryApplication interface {
	MakeTelemetry() telemetry.Telemetry
}

func Telemetry() telemetry.Telemetry {
	app, ok := App().(telemetryApplication)
	if !ok {
		return nil
	}
	return app.MakeTelemetry()
}
