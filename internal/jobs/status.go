// Copyright 2026 Bitwise Media Group Ltd.
// SPDX-License-Identifier: MIT

package jobs

import (
	"context"
	"fmt"
	"strconv"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ownedSelector matches every Job patchy created.
const ownedSelector = labelApp + "=" + appName + "," + labelManagedBy + "=" + managedBy

// Status reports a Job's state.
type Status struct {
	Active    int32
	Succeeded int32
	Failed    int32
	// Done means the Job reached a terminal condition (Complete or Failed).
	Done bool
}

// Status reports the named Job's state.
func (c *Client) Status(ctx context.Context, jobName string) (Status, error) {
	job, err := c.cs.BatchV1().Jobs(c.cfg.Namespace).Get(ctx, jobName, metav1.GetOptions{})
	if err != nil {
		return Status{}, fmt.Errorf("jobs: status of %s: %w", jobName, err)
	}
	return statusOf(job), nil
}

// Delete removes a Job and its pods (propagation: background).
func (c *Client) Delete(ctx context.Context, jobName string) error {
	policy := metav1.DeletePropagationBackground
	err := c.cs.BatchV1().Jobs(c.cfg.Namespace).Delete(ctx, jobName, metav1.DeleteOptions{
		PropagationPolicy: &policy,
	})
	if err != nil {
		return fmt.Errorf("jobs: delete %s: %w", jobName, err)
	}
	return nil
}

// Owned is one Job patchy created, with the issue it belongs to parsed back
// out of its labels and annotations.
type Owned struct {
	Name    string
	Repo    string
	Issue   int
	Attempt int
	Status  Status
}

// List returns the Jobs patchy owns in the namespace — used to reap orphans.
func (c *Client) List(ctx context.Context) ([]Owned, error) {
	list, err := c.cs.BatchV1().Jobs(c.cfg.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: ownedSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("jobs: list: %w", err)
	}
	owned := make([]Owned, 0, len(list.Items))
	for i := range list.Items {
		job := &list.Items[i]
		o := Owned{Name: job.Name, Status: statusOf(job)}
		// The annotation carries the true owner/name; the label value is
		// sanitized and only a fallback.
		if o.Repo = job.Annotations[annotationRepo]; o.Repo == "" {
			o.Repo = job.Labels[labelRepo]
		}
		o.Issue, _ = strconv.Atoi(job.Labels[labelIssue])
		o.Attempt, _ = strconv.Atoi(job.Labels[labelAttempt])
		owned = append(owned, o)
	}
	return owned, nil
}

func statusOf(job *batchv1.Job) Status {
	s := Status{
		Active:    job.Status.Active,
		Succeeded: job.Status.Succeeded,
		Failed:    job.Status.Failed,
	}
	for _, cond := range job.Status.Conditions {
		terminal := cond.Type == batchv1.JobComplete || cond.Type == batchv1.JobFailed
		if terminal && cond.Status == corev1.ConditionTrue {
			s.Done = true
		}
	}
	return s
}
