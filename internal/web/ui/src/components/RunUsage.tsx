// Agent-run accounting: the harness/model/tokens/cost rows the stage tabs
// embed in their run <dl>, and the always-present header badges that show
// the finding's cross-attempt totals without reflowing when data arrives.

import type { Finding, Usage } from "../types";
import { DASH, formatMicroUSD, formatTokens, totalTokens, usageBreakdown } from "../format";
import { Pill } from "./Pills";

export function RunAccountingRows({ harness, model, usage }: { harness?: string; model?: string; usage?: Usage }) {
  return (
    <>
      <div>
        <dt>Harness</dt>
        <dd>{harness ? <span class="ps-mono-tag">{harness}</span> : DASH}</dd>
      </div>
      <div>
        <dt>Model</dt>
        <dd>{model ? <span class="ps-mono-tag">{model}</span> : DASH}</dd>
      </div>
      <div>
        <dt>Tokens</dt>
        <dd>
          <span class="font-mono">{formatTokens(totalTokens(usage))}</span>
          {usageBreakdown(usage) ? (
            <small class="mt-0.5 block font-mono text-[10px] text-muted">{usageBreakdown(usage)}</small>
          ) : null}
        </dd>
      </div>
      <div>
        <dt>Cost</dt>
        <dd>
          <span class="font-mono">{formatMicroUSD(usage?.costMicroUSD || undefined)}</span>
        </dd>
      </div>
    </>
  );
}

// UsageBadges always renders both pills — a finding with no runs yet shows
// dashes, so the header keeps its shape when accounting lands later.
export function UsageBadges({ finding }: { finding: Finding }) {
  const u = finding.totalUsage;
  return (
    <>
      <Pill tone="neutral" label="Tokens" title={usageBreakdown(u) ?? "No agent runs accounted yet"}>
        {formatTokens(totalTokens(u))}
      </Pill>
      <Pill tone="neutral" label="Cost" title="Total across every attempt of both stages">
        {formatMicroUSD(u?.costMicroUSD || undefined)}
      </Pill>
    </>
  );
}
