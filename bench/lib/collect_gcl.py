#!/usr/bin/env python3
"""Poll Cloud Logging until all benchmark lines of a run have arrived, then save them.

Cloud Logging ingestion is delayed by seconds to minutes, so we poll until the number of
distinct messageIds reaches --expected (or --timeout). Output is gzip'd raw JSON; the
analysis scans it with a regex, so the exact payload layout does not matter.
"""
import argparse, gzip, re, subprocess, sys, time

EVENT_RE = re.compile(r"@@@ (\d+) (\S+) (\d+) (\d+) (\d+) (\d+) (\d+) (\S+) (\S+) @@@")


def fetch(project, flt, since, limit):
    full = f'{flt} AND timestamp>="{since}"'
    cmd = ["gcloud", "logging", "read", full, "--project", project, "--format=json",
           "--order=asc", f"--limit={limit}", "--quiet"]
    r = subprocess.run(cmd, capture_output=True, text=True)
    if r.returncode != 0:
        print("gcloud logging read failed:", r.stderr.strip()[:500], file=sys.stderr)
        return ""
    return r.stdout


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--project", required=True)
    ap.add_argument("--filter", required=True)
    ap.add_argument("--since", required=True, help="ISO timestamp (UTC) of run start")
    ap.add_argument("--expected", type=int, required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--timeout", type=int, default=900)
    ap.add_argument("--poll", type=int, default=20)
    ap.add_argument("--limit", type=int, default=2_000_000)
    ap.add_argument("--job-id", default="", help="Dataflow job id: stop early (exit 4) if the job failed or was cancelled")
    ap.add_argument("--region", default="")
    a = ap.parse_args()

    start = time.time()
    text, n = "", 0
    while True:
        text = fetch(a.project, a.filter, a.since, a.limit)
        n = len({m.group(1) for m in EVENT_RE.finditer(text)})
        print(f"[collect_gcl] {n}/{a.expected} events in Cloud Logging", file=sys.stderr)
        if n >= a.expected:
            break
        if a.job_id:
            st = subprocess.run(["gcloud", "dataflow", "jobs", "describe", a.job_id, "--project", a.project, "--region", a.region,
                                 "--format=value(currentState)"], capture_output=True, text=True).stdout.strip()
            if st in ("JOB_STATE_FAILED", "JOB_STATE_CANCELLED"):
                print(f"[collect_gcl] Dataflow job is {st} - stopping early", file=sys.stderr)
                with gzip.open(a.out, "wt") as f:
                    f.write(text)
                sys.exit(4)
        if time.time() - start > a.timeout:
            print("[collect_gcl] TIMEOUT - saving what we have", file=sys.stderr)
            break
        time.sleep(a.poll)

    with gzip.open(a.out, "wt") as f:
        f.write(text)
    sys.exit(0 if n >= a.expected else 3)


if __name__ == "__main__":
    main()
