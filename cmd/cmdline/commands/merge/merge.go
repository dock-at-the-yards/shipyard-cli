package mergecmd

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/dock-at-the-yards/shipyard-cli/cmd/cmdline/commands"
	"github.com/dock-at-the-yards/shipyard-core/common/types"
	shiperrs "github.com/dock-at-the-yards/shipyard-core/pkg/errors"
	"github.com/dock-at-the-yards/shipyard-core/pkg/results"

	"github.com/spf13/cobra"
)

var MergeCmd = &cobra.Command{
	Use:   "merge <results.json> [<results.json>...]",
	Short: "Merge sharded suite results into one verdict",
	Long: `Merge shard results.json files (from 'shipyard run --shard i/n') into a
single suite result. The merged verdict uses the same precedence as a normal
run (errored > flaked > failed > passed), so merged verdict == unsharded
verdict for shards of one suite.

Exit code 0 when files merge; nonzero on mismatched suites or duplicate evals.`,
	Args:         cobra.MinimumNArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		outFlag, _ := cmd.Flags().GetString("out")

		defer func() {
			_ = cmd.Flags().Set("out", "")
		}()

		suites := make([]*types.SuiteResults, 0, len(args))
		for _, path := range args {
			res, err := results.LoadSuiteResults(path)
			if err != nil {
				return err
			}
			suites = append(suites, res)
		}
		merged, err := Merge(suites...)
		if err != nil {
			return err
		}
		if strings.TrimSpace(outFlag) != "" {
			if err := results.SaveSuiteResults(outFlag, *merged); err != nil {
				return err
			}
			cmd.Printf("merged %d shard(s), %d eval(s), verdict %s -> %s\n",
				len(suites), merged.Totals.Evals, merged.Verdict, outFlag)
			return nil
		}
		data, err := json.MarshalIndent(merged, "", "  ")
		if err != nil {
			return err
		}
		commands.PrintStdout(cmd, string(data))
		return nil
	},
}

// Merge combines shard SuiteResults into one. All inputs must belong to the
// same suite (equal TargetName and SuiteDigest); eval names must be unique.
// Totals and the suite verdict are recomputed from the concatenated evals,
// mirroring the engine's fold (errored > flaked > failed > passed).
func Merge(suites ...*types.SuiteResults) (*types.SuiteResults, error) {
	if len(suites) == 0 {
		return nil, shiperrs.New(shiperrs.ErrorCodeInvalidArguments, "merge needs at least one results file")
	}
	base := suites[0]
	for _, s := range suites[1:] {
		if s.TargetName != base.TargetName {
			return nil, shiperrs.New(shiperrs.ErrorCodeInvalidArguments,
				fmt.Sprintf("cannot merge different suites: %q vs %q", base.TargetName, s.TargetName))
		}
		if s.SuiteDigest != "" && base.SuiteDigest != "" && s.SuiteDigest != base.SuiteDigest {
			return nil, shiperrs.New(shiperrs.ErrorCodeInvalidArguments,
				fmt.Sprintf("cannot merge runs with different suite digests: %q vs %q", base.SuiteDigest, s.SuiteDigest))
		}
	}

	seen := map[string]string{}
	evals := []types.EvalAggregate{}
	for _, s := range suites {
		for _, e := range s.Evals {
			if prev, dup := seen[e.EvalName]; dup {
				return nil, shiperrs.New(shiperrs.ErrorCodeInvalidArguments,
					fmt.Sprintf("duplicate eval %q in merge inputs (%s)", e.EvalName, prev))
			}
			seen[e.EvalName] = s.RunID
			evals = append(evals, e)
		}
	}
	sort.Slice(evals, func(a, b int) bool { return evals[a].EvalName < evals[b].EvalName })

	merged := &types.SuiteResults{
		SchemaVersion: "1",
		TargetType:    base.TargetType,
		TargetName:    base.TargetName,
		RunID:         base.RunID,
		SuiteDigest:   base.SuiteDigest,
		StartedAt:     base.StartedAt,
		EndedAt:       base.EndedAt,
	}
	allSameRun := true
	for _, s := range suites[1:] {
		if s.RunID != base.RunID {
			allSameRun = false
		}
		if s.StartedAt != "" && (merged.StartedAt == "" || s.StartedAt < merged.StartedAt) {
			merged.StartedAt = s.StartedAt
		}
		if s.EndedAt != "" && (merged.EndedAt == "" || s.EndedAt > merged.EndedAt) {
			merged.EndedAt = s.EndedAt
		}
	}
	if !allSameRun {
		merged.RunID = "merged"
	}
	merged.Evals = evals
	merged.Totals.Evals = len(evals)
	var usageList []*types.Usage
	var scoreSum float64
	for _, e := range evals {
		switch e.Verdict {
		case types.VerdictPassed:
			merged.Totals.Passed++
		case types.VerdictFailed:
			merged.Totals.Failed++
		case types.VerdictFlaked:
			merged.Totals.Flaked++
		case types.VerdictErrored:
			merged.Totals.Errored++
		default:
			merged.Totals.Failed++
		}
		if e.K > merged.K {
			merged.K = e.K
		}
		usageList = append(usageList, e.Usage)
		scoreSum += e.MeanScore
	}
	if len(evals) > 0 {
		merged.Totals.MeanScore = scoreSum / float64(len(evals))
	}
	merged.Totals.Usage = types.SumUsage(usageList)
	merged.Verdict = foldVerdict(evals)
	return merged, nil
}

// foldVerdict mirrors the engine's suite fold: errored > flaked > failed.
func foldVerdict(evals []types.EvalAggregate) types.Verdict {
	verdict := types.VerdictPassed
	for _, e := range evals {
		switch e.Verdict {
		case types.VerdictErrored:
			return types.VerdictErrored
		case types.VerdictFlaked:
			if verdict != types.VerdictErrored {
				verdict = types.VerdictFlaked
			}
		case types.VerdictFailed:
			if verdict == types.VerdictPassed {
				verdict = types.VerdictFailed
			}
		}
	}
	return verdict
}

func init() {
	MergeCmd.Flags().String("out", "", "Write merged results JSON to this path (default: print to stdout)")
	commands.Register(MergeCmd)
}
