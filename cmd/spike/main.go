// Package main is the platform foundations spike for Kaimi.
//
// Validates that Google ADK Go SDK can reach Gemini via Vertex AI (BackendEnterprise)
// using Application Default Credentials.
//
// Usage:
//
//	go run ./cmd/spike run --prompt "Hello from Kaimi!"
//	go run ./cmd/spike console
package main

import (
	"context"
	"log"
	"os"

	"google.golang.org/genai"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/model/gemini"
)

const (
	// modelName is the Gemini model to use. Architecture targets Gemini 3 Pro;
	// gemini-2.5-pro is the current stable "pro" model on Vertex AI.
	// TODO(phase-1): upgrade to gemini-3-pro once GA on Vertex AI us-east4.
	modelName = "gemini-2.5-pro"

	projectID = "kaimi-seeker"
	location  = "us-east4"
)

func main() {
	ctx := context.Background()

	// BackendEnterprise = Vertex AI / Gemini Enterprise Agent Platform.
	// Uses Application Default Credentials — run `gcloud auth application-default login` once.
	model, err := gemini.NewModel(ctx, modelName, &genai.ClientConfig{
		Backend:  genai.BackendEnterprise,
		Project:  projectID,
		Location: location,
	})
	if err != nil {
		log.Fatalf("failed to create Gemini model: %v", err)
	}

	a, err := llmagent.New(llmagent.Config{
		Name:        "kaimi_hello",
		Model:       model,
		Description: "Platform validation agent. Use to confirm ADK + Vertex AI connectivity.",
		Instruction: "You are a helpful assistant. When greeted, respond with: 'Kaimi platform OK — ADK agent running on Vertex AI.'",
	})
	if err != nil {
		log.Fatalf("failed to create agent: %v", err)
	}

	config := &launcher.Config{
		AgentLoader: agent.NewSingleLoader(a),
	}

	l := full.NewLauncher()
	if err = l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("launcher failed: %v\n\nUsage:\n%s", err, l.CommandLineSyntax())
	}
}
