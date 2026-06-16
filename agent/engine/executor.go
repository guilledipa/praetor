package engine

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/guilledipa/praetor/agent/facts"
	"github.com/guilledipa/praetor/agent/resources"
	pkgbroker "github.com/guilledipa/praetor/pkg/broker"
	masterpb "github.com/guilledipa/praetor/proto/gen/master"
	"github.com/nats-io/nats.go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
)

// Catalog represents the structure of the catalog received from the master.
type Catalog struct {
	APIVersion string            `json:"apiVersion"`
	Kind       string            `json:"kind"`
	Metadata   map[string]string `json:"metadata"`
	Spec       struct {
		Resources []json.RawMessage `json:"resources"`
	} `json:"spec"`
}

// CatalogResource represents a single resource entry in the catalog.
type CatalogResource struct {
	Type string          `json:"type"`
	Spec json.RawMessage `json:"spec"`
}

type Executor struct {
	NodeID       string
	NatsConn     *nats.Conn
	MasterPubKey ed25519.PublicKey
	Logger       *slog.Logger
	DryRun       bool
}

var isRunning atomic.Bool

func (e *Executor) FetchAndApplyCatalog(ctx context.Context) {
	if !isRunning.CompareAndSwap(false, true) {
		e.Logger.Warn("Configuration check already in progress, skipping trigger")
		return
	}
	defer isRunning.Store(false)

	ctx, span := otel.Tracer("agent-engine").Start(ctx, "FetchAndApplyCatalog")
	defer span.End()

	e.Logger.Info("--- Running Configuration Check ---")

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	agentFacts := facts.Collect()
	stringFacts := make(map[string]string)
	for k, v := range agentFacts {
		stringFacts[k] = fmt.Sprintf("%v", v)
	}
	e.Logger.Info("Collected facts", "facts", stringFacts)

	req := pkgbroker.CatalogRequest{
		NodeID: e.NodeID,
		Facts:  stringFacts,
	}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to marshal catalog request")
		e.Logger.Error("Failed to marshal catalog request", "error", err)
		return
	}

	natsResp, err := e.NatsConn.RequestWithContext(ctx, "agent.catalog.request."+e.NodeID, reqBytes)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to get catalog via NATS")
		e.Logger.Error("Error fetching catalog from master via NATS", "error", err)
		return
	}

	var catResp pkgbroker.CatalogResponse
	if err := json.Unmarshal(natsResp.Data, &catResp); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "Failed to unmarshal NATS catalog response")
		e.Logger.Error("Error unmarshalling catalog response", "error", err)
		return
	}

	catalogContent := catResp.Catalog
	signature := catResp.Signature

	// Verify signature
	if len(signature) == 0 {
		e.Logger.Warn("No signature found in catalog response")
		return
	}

	if !ed25519.Verify(e.MasterPubKey, []byte(catalogContent), signature) {
		span.SetStatus(codes.Error, "Catalog signature failed verification")
		e.Logger.Error("Catalog signature verification failed!")
		return
	}
	e.Logger.Info("Catalog signature verified successfully.")

	var catalog Catalog
	if err := json.Unmarshal([]byte(catalogContent), &catalog); err != nil {
		e.Logger.Error("Error unmarshalling catalog JSON", "error", err)
		return
	}
	e.Logger.Info("Successfully fetched and parsed catalog", "resource_count", len(catalog.Spec.Resources))

	allCompliant := true
	var reports []*masterpb.ResourceReport

	// 1. Inflate all resources into memory first
	var unsortedResources []resources.Resource
	for _, resData := range catalog.Spec.Resources {
		var typeMeta struct {
			Kind string `json:"kind"`
		}
		if err := json.Unmarshal(resData, &typeMeta); err != nil {
			e.Logger.Error("Error unmarshalling resource kind", "error", err)
			reports = append(reports, &masterpb.ResourceReport{
				Type:      "Unknown",
				Compliant: false,
				Message:   fmt.Sprintf("Failed to unmarshal kind: %v", err),
			})
			allCompliant = false
			continue
		}

		res, err := resources.CreateResource(typeMeta.Kind, resData)
		if err != nil {
			e.Logger.Error("Error creating resource instance", "kind", typeMeta.Kind, "error", err)
			reports = append(reports, &masterpb.ResourceReport{
				Type:      typeMeta.Kind,
				Compliant: false,
				Message:   fmt.Sprintf("Failed to create instance: %v", err),
			})
			allCompliant = false
			continue
		}
		unsortedResources = append(unsortedResources, res)
	}

	// 2. Initialize and Run the Scheduler
	sched, err := newScheduler(unsortedResources, e.Logger)
	if err != nil {
		e.Logger.Error("Failed to initialize DAG scheduler", "error", err)
		return
	}

	var mu sync.Mutex
	concurrency := 4 // Default concurrency level

	applyFunc := func(ctx context.Context, res resources.Resource) (bool, string, error) {
		selfKey := fmt.Sprintf("%s[%s]", res.Type(), res.ID())
		_, resSpan := otel.Tracer("agent-engine").Start(ctx, fmt.Sprintf("Apply %s", selfKey))
		defer resSpan.End()

		resLogger := e.Logger.With("resource_type", res.Type(), "resource_id", res.ID())

		report := &masterpb.ResourceReport{
			Type:      res.Type(),
			Id:        res.ID(),
			Compliant: true,
			Message:   "Resource is compliant",
		}

		driftDetected := false

		defer func() {
			mu.Lock()
			reports = append(reports, report)
			if !report.Compliant || driftDetected {
				allCompliant = false
			}
			mu.Unlock()
		}()

		currentState, err := res.Get()
		if err != nil {
			resLogger.Error("Error getting state", "error", err)
			report.Compliant = false
			report.Message = fmt.Sprintf("Error getting state: %v", err)
			resSpan.SetStatus(codes.Error, report.Message)
			return false, report.Message, err
		}

		isCompliant, err := res.Test(currentState)
		if err != nil {
			resLogger.Error("Error testing state", "error", err)
			report.Compliant = false
			report.Message = fmt.Sprintf("Error testing state: %v", err)
			resSpan.SetStatus(codes.Error, report.Message)
			return false, report.Message, err
		}

		if !isCompliant {
			driftDetected = true
			report.Compliant = false
			if e.DryRun {
				report.Message = "Drift detected (Simulation / Dry-Run)"
				resLogger.Info("Simulation: Drift detected, skipping enforcement due to dry-run mode")
				return false, report.Message, nil
			} else {
				resLogger.Info("Drift detected. Enforcing desired state...")
				err := res.Set()
				if err != nil {
					report.Message = fmt.Sprintf("Failed to enforce: %v", err)
					resLogger.Error("Error setting state", "error", err)
					resSpan.SetStatus(codes.Error, report.Message)
					return false, report.Message, err
				} else {
					report.Message = "Drift detected but successfully enforced state"
					report.Compliant = true
					resLogger.Info("Successfully enforced state")
					return true, report.Message, nil
				}
			}
		}

		resLogger.Info("Resource is compliant", "type", res.Type(), "id", res.ID())
		return true, report.Message, nil
	}

	sched.run(ctx, concurrency, applyFunc)

	// Append skipped reports dynamically
	for name, state := range sched.states {
		if state == StateSkipped {
			res := sched.resMap[name]
			reports = append(reports, &masterpb.ResourceReport{
				Type:      res.Type(),
				Id:        res.ID(),
				Compliant: false,
				Message:   "Skipped due to failed parent dependencies",
			})
			allCompliant = false
		}
	}

	e.Logger.Info("--- Configuration Check Finished ---")

	// Send the state report via NATS Request-Reply
	var reportsList []*pkgbroker.ResourceReport
	for _, r := range reports {
		reportsList = append(reportsList, &pkgbroker.ResourceReport{
			Type:      r.Type,
			Id:        r.Id,
			Compliant: r.Compliant,
			Message:   r.Message,
		})
	}

	reportReq := pkgbroker.StateReportRequest{
		NodeID:      e.NodeID,
		Resources:   reportsList,
		IsCompliant: allCompliant,
		Timestamp:   time.Now().Unix(),
	}

	reportBytes, err := json.Marshal(reportReq)
	if err != nil {
		e.Logger.Error("Failed to marshal state report request", "error", err)
		return
	}

	reportResp, err := e.NatsConn.RequestWithContext(ctx, "agent.state.report."+e.NodeID, reportBytes)
	if err != nil {
		e.Logger.Error("Failed to report state to master via NATS", "error", err)
	} else {
		var stateResp pkgbroker.StateReportResponse
		if err := json.Unmarshal(reportResp.Data, &stateResp); err == nil && stateResp.Acknowledged {
			e.Logger.Info("Successfully reported state to master via NATS")
		} else {
			e.Logger.Error("NATS state report acknowledgment failed")
		}
	}
}
