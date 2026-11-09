#!/usr/bin/env python3
"""Release-time check: the embedded changelog snapshot must contain the merged PRs.

Run from a checkout with authenticated gh: python3 scripts/check-changelog.py
Fails when more than --allow merged PRs (default 1, the PR that regenerates the
snapshot cannot list itself) are missing from web/changelog.json.
Fix: python3 scripts/build-changelog.py --repo <Repo>
"""
import argparse
import json
import pathlib
import subprocess
import sys


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--allow", type=int, default=1)
    args = parser.parse_args()
    root = pathlib.Path(__file__).resolve().parents[1]
    snapshot = json.loads((root / "web/changelog.json").read_text())
    (repo,) = snapshot["sources"]
    have = {e["id"] for e in snapshot["entries"] if e["kind"] == "pr"}
    rows = subprocess.check_output(
        ["gh", "api", "--paginate", "--jq", '.[] | select(.merged_at != null and .base.ref == "main") | .number',
         f"repos/navi-crwn/{repo}/pulls?state=closed&base=main&per_page=100"], text=True).split()
    missing = sorted((int(n) for n in rows if f"#{n}" not in have), reverse=True)
    if len(missing) > args.allow:
        print(f"web/changelog.json is stale: merged PRs missing: {', '.join('#%d' % n for n in missing)}")
        print(f"run: python3 scripts/build-changelog.py --repo {repo}")
        return 1
    print(f"changelog snapshot ok ({len(missing)} newer PR(s) not yet listed)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
