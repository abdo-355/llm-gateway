package verification

import (
	"fmt"
	"io"
	"sort"
	"text/tabwriter"
	"time"
)

func PrintReport(w io.Writer, report *Report) {
	if report == nil {
		return
	}

	passed, failed, skipped := 0, 0, 0
	for _, result := range report.Results {
		switch result.Status {
		case "PASS":
			passed++
		case "FAIL":
			failed++
		case "SKIP":
			skipped++
		}
	}

	fmt.Fprintf(w, "Upstream Model Verification Report\n")
	fmt.Fprintf(w, "Started: %s\n", report.StartedAt.Format(time.RFC3339))
	fmt.Fprintf(w, "Finished: %s\n", report.EndedAt.Format(time.RFC3339))
	fmt.Fprintf(w, "Duration: %s\n", report.EndedAt.Sub(report.StartedAt).Round(time.Second))
	fmt.Fprintf(w, "Total Probes: %d\n", len(report.Results))
	fmt.Fprintf(w, "Passed: %d\n", passed)
	fmt.Fprintf(w, "Failed: %d\n", failed)
	fmt.Fprintf(w, "Skipped: %d\n\n", skipped)

	printProviderSummary(w, report)
	printFeatureSummary(w, report)
	printSkipped(w, report)
	printFailures(w, report)
}

func printProviderSummary(w io.Writer, report *Report) {
	summaries := providerSummaries(report)
	fmt.Fprintf(w, "Provider Summary\n")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PROVIDER\tMODELS\tPASSED\tFAILED\tSKIPPED\tTOTAL")
	for _, summary := range summaries {
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\t%d\n", summary.Provider, summary.Models, summary.Passed, summary.Failed, summary.Skipped, summary.Total)
	}
	_ = tw.Flush()
	fmt.Fprintln(w)
}

func printFeatureSummary(w io.Writer, report *Report) {
	summaries := featureSummaries(report)
	fmt.Fprintf(w, "Feature Summary\n")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PROBE\tPASSED\tFAILED\tSKIPPED\tTOTAL")
	for _, summary := range summaries {
		_, _ = fmt.Fprintf(tw, "%s\t%d\t%d\t%d\t%d\n", summary.Probe, summary.Passed, summary.Failed, summary.Skipped, summary.Total)
	}
	_ = tw.Flush()
	fmt.Fprintln(w)
}

func printFailures(w io.Writer, report *Report) {
	failures := make([]ProbeResult, 0)
	for _, result := range report.Results {
		if result.Status == "FAIL" {
			failures = append(failures, result)
		}
	}

	fmt.Fprintf(w, "Failures\n")
	if len(failures) == 0 {
		fmt.Fprintf(w, "None\n")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PROVIDER\tMODEL\tENDPOINT\tPROBE\tHTTP\tLATENCY\tTOKENS\tFAILURE")
	for _, result := range failures {
		_, _ = fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n",
			result.Provider,
			result.Model,
			result.Endpoint,
			result.Probe,
			result.HTTPStatus,
			result.Latency.Round(time.Millisecond),
			emptyDash(result.TokensUsed),
			result.Failure,
		)
	}
	_ = tw.Flush()
}

func printSkipped(w io.Writer, report *Report) {
	skips := make([]ProbeResult, 0)
	for _, result := range report.Results {
		if result.Status == "SKIP" && shouldPrintSkip(result) {
			skips = append(skips, result)
		}
	}

	fmt.Fprintf(w, "Skipped\n")
	if len(skips) == 0 {
		fmt.Fprintf(w, "None\n\n")
		return
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "PROVIDER\tMODEL\tENDPOINT\tPROBE\tHTTP\tFAILURE")
	for _, result := range skips {
		_, _ = fmt.Fprintf(
			tw,
			"%s\t%s\t%s\t%s\t%d\t%s\n",
			result.Provider,
			result.Model,
			result.Endpoint,
			result.Probe,
			result.HTTPStatus,
			result.Failure,
		)
	}
	_ = tw.Flush()
	fmt.Fprintln(w)
}

func providerSummaries(report *Report) []providerSummary {
	lookup := make(map[string]*providerSummary)
	modelSets := make(map[string]map[string]struct{})

	for _, result := range report.Results {
		summary, ok := lookup[result.Provider]
		if !ok {
			summary = &providerSummary{Provider: result.Provider}
			lookup[result.Provider] = summary
			modelSets[result.Provider] = make(map[string]struct{})
		}
		modelSets[result.Provider][result.Model] = struct{}{}
		summary.Total++
		switch result.Status {
		case "PASS":
			summary.Passed++
		case "FAIL":
			summary.Failed++
		case "SKIP":
			summary.Skipped++
		}
	}

	summaries := make([]providerSummary, 0, len(lookup))
	for provider, summary := range lookup {
		summary.Models = len(modelSets[provider])
		summaries = append(summaries, *summary)
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Provider < summaries[j].Provider })
	return summaries
}

func featureSummaries(report *Report) []featureSummary {
	lookup := make(map[string]*featureSummary)
	for _, result := range report.Results {
		summary, ok := lookup[result.Probe]
		if !ok {
			summary = &featureSummary{Probe: result.Probe}
			lookup[result.Probe] = summary
		}
		summary.Total++
		switch result.Status {
		case "PASS":
			summary.Passed++
		case "FAIL":
			summary.Failed++
		case "SKIP":
			summary.Skipped++
		}
	}

	summaries := make([]featureSummary, 0, len(lookup))
	for _, summary := range lookup {
		summaries = append(summaries, *summary)
	}
	sort.Slice(summaries, func(i, j int) bool { return summaries[i].Probe < summaries[j].Probe })
	return summaries
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func shouldPrintSkip(result ProbeResult) bool {
	return result.Failure != "" && result.Failure != "not applicable for configured provider capabilities"
}
