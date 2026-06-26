package hunttrigger

import (
	"context"
	"fmt"
	"log"

	run "cloud.google.com/go/run/apiv2"
	"cloud.google.com/go/run/apiv2/runpb"
)

// CloudRunJob is a Runner that starts a Cloud Run Job execution — the existing
// Hunter→Scorer→Queue pipeline — via the Cloud Run Admin API. It is fire-and-forget: it
// returns once the execution has been STARTED, not when the hunt finishes. The pipeline
// runs server-side as ITS OWN service account (the API only needs run.jobs.run on the job),
// and re-runs are bounded by the Trigger's minInterval, so we don't hold a connection open
// for the multi-minute hunt. The job's own env/config (search window, NAICS) drives the
// hunt; nothing about the tenant is passed here.
type CloudRunJob struct {
	name string
	// execute starts the job execution. Split out so tests exercise Run without GCP.
	execute func(ctx context.Context, name string) error
}

// NewCloudRunJob connects to the Cloud Run Admin API via ADC and targets one job. The
// resource name is projects/{project}/locations/{region}/jobs/{job}. It does not run the
// job — call Run (typically via a Trigger) for that.
func NewCloudRunJob(ctx context.Context, projectID, region, job string) (*CloudRunJob, error) {
	if projectID == "" || region == "" || job == "" {
		return nil, fmt.Errorf("hunttrigger: project, region, and job are all required")
	}
	client, err := run.NewJobsClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("hunttrigger: cloud run jobs client: %w", err)
	}
	name := fmt.Sprintf("projects/%s/locations/%s/jobs/%s", projectID, region, job)

	return &CloudRunJob{
		name: name,
		execute: func(ctx context.Context, name string) error {
			op, err := client.RunJob(ctx, &runpb.RunJobRequest{Name: name})
			if err != nil {
				return err
			}
			// Fire-and-forget: the execution is running server-side. Log its operation so
			// an operator can correlate with the job's own logs without us blocking here.
			log.Printf("hunttrigger: started pipeline job execution (%s)", op.Name())
			return nil
		},
	}, nil
}

// Run starts one pipeline job execution. Errors are wrapped with the job name (which carries
// no secret).
func (j *CloudRunJob) Run(ctx context.Context) error {
	if err := j.execute(ctx, j.name); err != nil {
		return fmt.Errorf("hunttrigger: run job %s: %w", j.name, err)
	}
	return nil
}
