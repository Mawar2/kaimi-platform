// Command kaimi-key is the operator CLI for Kaimi product keys: mint a
// time-limited access key for a tester, revoke one, or list all. Keys live in
// the licensing project's Firestore (the same store the access gate validates).
//
// Usage:
//
//	kaimi-key -project <licensing-project> mint   --tester "Ey3 Technologies" --days 14 [--url https://ey3-kaimi...run.app]
//	kaimi-key -project <licensing-project> revoke KAIMI-7F3A-9C2E-B1D4
//	kaimi-key -project <licensing-project> list
//
// Auth is ADC (run `gcloud auth application-default login` or use a service
// account). The -project flag defaults to $GCP_PROJECT_ID.
//
// `cost <key>` (per-key spend) ships with the P3 token-metering work.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Mawar2/Kaimi/internal/productkey"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "kaimi-key:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	global := flag.NewFlagSet("kaimi-key", flag.ContinueOnError)
	project := global.String("project", os.Getenv("GCP_PROJECT_ID"), "GCP project holding the product-key Firestore (default $GCP_PROJECT_ID)")
	global.Usage = usage
	// Parse only up to the subcommand: find the first non-flag arg.
	if err := global.Parse(args); err != nil {
		return err
	}
	rest := global.Args()
	if len(rest) == 0 {
		usage()
		return errors.New("a subcommand is required (mint|revoke|list)")
	}
	if *project == "" {
		return errors.New("-project (or $GCP_PROJECT_ID) is required")
	}

	ctx := context.Background()
	reg, err := productkey.NewFirestoreRegistry(ctx, *project)
	if err != nil {
		return err
	}
	defer func() { _ = reg.Close() }()

	switch rest[0] {
	case "mint":
		return cmdMint(ctx, reg, rest[1:])
	case "revoke":
		return cmdRevoke(ctx, reg, rest[1:])
	case "list":
		return cmdList(ctx, reg)
	default:
		usage()
		return fmt.Errorf("unknown subcommand %q", rest[0])
	}
}

func cmdMint(ctx context.Context, reg productkey.Registry, args []string) error {
	fs := flag.NewFlagSet("mint", flag.ContinueOnError)
	tester := fs.String("tester", "", "tester label, e.g. \"Ey3 Technologies\" (required)")
	days := fs.Int("days", 14, "access window in days")
	baseURL := fs.String("url", "", "optional deploy base URL; if set, prints a ready magic link")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *tester == "" {
		return errors.New("mint: --tester is required")
	}
	if *days <= 0 {
		return errors.New("mint: --days must be positive")
	}
	rec, err := reg.Mint(ctx, *tester, time.Duration(*days)*24*time.Hour)
	if err != nil {
		return err
	}
	fmt.Printf("Product key:  %s\n", rec.Key)
	fmt.Printf("Tester:       %s\n", rec.Tester)
	fmt.Printf("Expires:      %s (%d days)\n", rec.ExpiresAt.UTC().Format(time.RFC3339), *days)
	if *baseURL != "" {
		fmt.Printf("Magic link:   %s/access?key=%s\n", strings.TrimRight(*baseURL, "/"), rec.Key)
	}
	return nil
}

func cmdRevoke(ctx context.Context, reg productkey.Registry, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: kaimi-key revoke <KAIMI-XXXX-XXXX-XXXX>")
	}
	if err := reg.Revoke(ctx, args[0]); err != nil {
		if errors.Is(err, productkey.ErrNotFound) {
			return fmt.Errorf("no such key: %s", args[0])
		}
		return err
	}
	fmt.Printf("Revoked %s\n", productkey.Normalize(args[0]))
	return nil
}

func cmdList(ctx context.Context, reg productkey.Registry) error {
	recs, err := reg.List(ctx)
	if err != nil {
		return err
	}
	if len(recs) == 0 {
		fmt.Println("(no product keys)")
		return nil
	}
	now := time.Now()
	tw := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "KEY\tTESTER\tEXPIRES\tSTATUS")
	for i := range recs {
		r := &recs[i]
		status := "active"
		switch {
		case r.Revoked:
			status = "revoked"
		case !r.Valid(now):
			status = "expired"
		}
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", r.Key, r.Tester, r.ExpiresAt.UTC().Format("2006-01-02"), status)
	}
	return tw.Flush()
}

func usage() {
	fmt.Fprint(os.Stderr, `kaimi-key — manage Kaimi product keys (access credentials)

Usage:
  kaimi-key -project <gcp-project> mint   --tester "Name" --days 14 [--url https://deploy.run.app]
  kaimi-key -project <gcp-project> revoke KAIMI-XXXX-XXXX-XXXX
  kaimi-key -project <gcp-project> list

-project defaults to $GCP_PROJECT_ID. Auth via ADC.
`)
}
