package engine

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/guilledipa/praetor/agent/facts"
	"github.com/guilledipa/praetor/agent/resources"
	masterpb "github.com/guilledipa/praetor/proto/gen/master"
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
	MasterClient masterpb.ConfigurationMasterClient
	MasterPubKey ed25519.PublicKey
	Logger       *slog.Logger
}

func (e *Executor) FetchAndApplyCatalog() {
	e.Logger.Info("--- Running Configuration Check ---")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentFacts := facts.Collect()
	stringFacts := make(map[string]string)
	for k, v := range agentFacts {
		stringFacts[k] = fmt.Sprintf("%v", v)
	}
	e.Logger.Info("Collected facts", "facts", stringFacts)

	resp, err := e.MasterClient.GetCatalog(ctx, &masterpb.GetCatalogRequest{NodeId: e.NodeID, Facts: stringFacts})
	if err != nil {
		e.Logger.Error("Error fetching catalog from master", "error", err)
		return
	}

	catalogContent := resp.GetCatalog().GetContent()
	signature := resp.GetSignature()

	// Verify signature
	if len(signature) == 0 {
		e.Logger.Warn("No signature found in catalog response")
		return
	}

	if !ed25519.Verify(e.MasterPubKey, []byte(catalogContent), signature) {
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

	// 2. Compute Directed Acyclic Graph Topo Sort
	sortedResources, err := buildDAG(unsortedResources)
	if err != nil {
		e.Logger.Error("DAG Resolution Failed", "error", err)
		return
	}
	e.Logger.Info("DAG built successfully", "nodes", len(sortedResources))

	// 3. Linearly Enforce the Sorted DAG
	failedNodes := make(map[string]bool)
	for _, res := range sortedResources {
		selfKey := fmt.Sprintf("%s[%s]", res.Type(), res.ID())

		report := &masterpb.ResourceReport{
			Type:      res.Type(),
			Id:        res.ID(),
			Compliant: true,
			Message:   "Resource is compliant",
		}

		resLogger := e.Logger.With("resource_type", res.Type(), "resource_id", res.ID())

		// Pre-Execution Dependency Check
		skip := false
		skipReason := ""
		allDeps := append(res.Requires(), res.Before()...)
		for _, dep := range allDeps {
			depKey := fmt.Sprintf("%s[%s]", dep.Kind, dep.Name)
			if failedNodes[depKey] {
				skip = true
				skipReason = fmt.Sprintf("Skipped: Dependency %s failed", depKey)
				break
			}
		}

		if skip {
			resLogger.Warn("Skipping resource due to failed dependency", "reason", skipReason)
			report.Compliant = false
			report.Message = skipReason
			failedNodes[selfKey] = true
			reports = append(reports, report)
			allCompliant = false
			continue
		}

		currentState, err := res.Get()
		if err != nil {
			resLogger.Error("Error getting state", "error", err)
			report.Compliant = false
			report.Message = fmt.Sprintf("Error getting state: %v", err)
			reports = append(reports, report)
			allCompliant = false
			failedNodes[selfKey] = true
			continue
		}

		isCompliant, err := res.Test(currentState)
		if err != nil {
			resLogger.Error("Error testing state", "error", err)
			report.Compliant = false
			report.Message = fmt.Sprintf("Error testing state: %v", err)
			reports = append(reports, report)
			allCompliant = false
			failedNodes[selfKey] = true
			continue
		}

		if !isCompliant {
			allCompliant = false
			report.Compliant = false
			resLogger.Info("Drift detected. Enforcing desired state...")
			err := res.Set()
			if err != nil {
				report.Message = fmt.Sprintf("Failed to enforce: %v", err)
				resLogger.Error("Error setting state", "error", err)
				failedNodes[selfKey] = true
			} else {
				report.Message = "Drift detected but successfully enforced state"
				report.Compliant = true
				resLogger.Info("Successfully enforced state")
			}
		} else {
			resLogger.Info("Resource is compliant", "type", res.Type(), "id", res.ID())
		}
		reports = append(reports, report)
	}

	e.Logger.Info("--- Configuration Check Finished ---")

	// Send the state report
	_, err = e.MasterClient.ReportState(ctx, &masterpb.ReportStateRequest{
		NodeId:      e.NodeID,
		Resources:   reports,
		IsCompliant: allCompliant,
		Timestamp:   time.Now().Unix(),
	})
	if err != nil {
		e.Logger.Error("Failed to report state to master", "error", err)
	} else {
		e.Logger.Info("Successfully reported state to master")
	}
}
