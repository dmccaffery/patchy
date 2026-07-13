// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package telemetry

import (
	"context"
	"fmt"

	"go.opentelemetry.io/contrib/exporters/autoexport"
)

// envExporters builds each signal's exporter from the OTEL_* environment via
// autoexport, which honors OTEL_TRACES_EXPORTER / OTEL_METRICS_EXPORTER /
// OTEL_LOGS_EXPORTER and the OTLP endpoint/protocol variables. It is the path
// for exporting to a collector in-cluster; a configured directory bypasses it.
func envExporters(ctx context.Context) (signalExporters, error) {
	span, err := autoexport.NewSpanExporter(ctx)
	if err != nil {
		return signalExporters{}, fmt.Errorf("trace exporter: %w", err)
	}
	reader, err := autoexport.NewMetricReader(ctx)
	if err != nil {
		return signalExporters{}, fmt.Errorf("metric reader: %w", err)
	}
	logs, err := autoexport.NewLogExporter(ctx)
	if err != nil {
		return signalExporters{}, fmt.Errorf("log exporter: %w", err)
	}
	return signalExporters{span: span, reader: reader, logs: logs}, nil
}
