# How a check works

## Which files count

A check compares the paths matching `docs` in the config (git pathspecs) between the pull request's merge base and its head: exactly what the pull request changed, not what landed on main since it branched. Symlinks and submodules under those paths are skipped and listed in the comment. A pull request that changes none of them gets no check.

## The two builds

Each side becomes a SelfBench repo ablation: a Bash script SelfBench runs in `/app` as it builds each task's image, before the agent starts.

- **before** removes the changed files the merge base lacks, and writes the rest as the merge base has them.
- **after** does the same with the pull request's head.

Files outside the change stay as each task has them, so the two sides differ by the docs change alone. A task keeps its own code, so a pull request that also changes code is checked on its docs only.

The files travel in the script as a compressed archive, and SelfBench takes scripts up to 64 KB, so a very large docs change is refused. Names look like `dari-docs #12 after 1a2b3c4d5e`: the hash is of the script, so a push that leaves the docs as they were reuses the build. `dari-docs check --dry-run` prints both scripts.

SelfBench builds every approved task in each ablation. A task that fails to build on either side is left out, with the reason, in the comment.

## The run

One SelfBench comparison runs every configured model on every task built on both sides, under each side, with `baseline: false`. Its ID comes from the repository, the pull request, both scripts, and the models and tasks, so rerunning a job reads the same comparison instead of starting another (`--fresh` starts a new one).

## The comment

The bot keeps one comment per pull request and edits it as the check goes. It carries the comparison's ID for the next run, which stops that comparison when a newer push starts its own. Only a comment the bot wrote is read back.

The result lists, for each model and harness, the tasks passed before and after, the change in percentage points, and cost; then the tasks whose result changed, the tasks that fail on both sides, trials that errored, and tasks left out. Failure explanations are SelfBench's.

## Cleanup

When the pull request closes, the action stops its comparison and deletes every ablation named `dari-docs #<number> …`. `dari-docs cleanup --pr N` does the same by hand.
