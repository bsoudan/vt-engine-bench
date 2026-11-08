#!/usr/bin/env python3
"""Turn headless benchmark runs into the README's table.

    cargo run --release -- --csv results/<date>-headless-<machine>.csv --run 1
    (and --run 2, --run 3)
    python3 scripts/summarize_headless.py results/<date>-headless-<machine>.csv

The CSV is what the benchmark appends (run,workload,engine,median_ms); the
table has one row per workload and one column per engine, each cell the
median of the runs, the lowest in a row in bold.
"""
import csv
import statistics
import sys
from collections import OrderedDict

LABELS = {"plain-scroll": "plain scroll, ms", "ansi-scroll": "ANSI scroll, ms", "tui-redraw": "TUI redraw, ms",
          "tmux-client": "tmux client (repeated recording), ms"}


def main(path):
    rows = list(csv.DictReader(open(path)))
    engines = list(OrderedDict.fromkeys(r["engine"] for r in rows))
    workloads = list(OrderedDict.fromkeys(r["workload"] for r in rows))
    runs = sorted({r["run"] for r in rows})
    print(f"{len(runs)} runs: {', '.join(runs)}\n")
    print("| Workload | " + " | ".join(engines) + " |")
    print("|---|" + "---:|" * len(engines))
    for workload in workloads:
        medians = []
        for engine in engines:
            samples = [float(r["median_ms"]) for r in rows if r["workload"] == workload and r["engine"] == engine]
            if len(samples) != len(runs):
                sys.exit(f"{workload}/{engine}: {len(samples)} samples for {len(runs)} runs")
            medians.append(statistics.median(samples))
        best = min(medians)
        cells = [f"**{m:.2f}**" if m == best else f"{m:.2f}" for m in medians]
        print(f"| {LABELS.get(workload, workload)} | " + " | ".join(cells) + " |")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        sys.exit(__doc__)
    main(sys.argv[1])
