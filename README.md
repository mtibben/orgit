# grit

`grit` is a tool for dealing with multiple git repos on the command line.

`grit` differs from other tools in that it:
  * has minimalistic config, so you can use it immediately with sensible defaults
  * relies on the git CLI for all git operations, so all config that applies to git is respected
  * uses concurrency wherever possible, so it's fast
  * supports nested trees of git repositories, like you find in Gitlab Groups
  * ~uses globbing for repo selection~

Features
 - `grit get` repos from GitHub or Gitlab by using the project URL without the proceeding https:// or git@
 - `@latest` = the tag with the highest semver version

## Example use

```shell
export GRIT_WORKSPACE=~/Developer/src          # Set the current directory as the grit workspace. The git workspace is a directory
                                               # that mirrors the URL structure remote git repositories.
grit get github.com/<ORG-NAME>/<PROJECT-NAME>  # Clone a repo from GitHub or Gitlab using the project URL. The repo will be cloned
                                               #   into $GRIT_WORKSPACE/github.com/<ORG-NAME>/<PROJECT-NAME>
grit sync github.com/<ORG-NAME>                # Clone all remote repos from a GitHub or Gitlab org in parallel.
grit sync github.com//<ORG-NAME> --archive     #   --archive:    Move local repos that are not found remotely to `$GRIT_WORKSPACE/.archive/`
grit sync --pristine                            #   --pristine:   Restore local repos to pristine state with stash, reset, clean
grit get --pristine --except-for my-repo       #   --except-for: Except for repos that match the 'my-repo' glob
grit list                                      # List all local repos in the workspace
grit status                                    # Show the status of all local repos that have changes
```

## Configuration

- `GRIT_WORKSPACE` can be set to a directory
- Configure your `.git/config` and `.netrc` for git and auth config

A useful alias for changing directory to a repo is:
```
alias gritcd="cd \$(grit list --full-path | fzf) && pwd"
```

## Prior art
 - [git-repo (aka `repo`)](https://gerrit.googlesource.com/git-repo)
 - [`ghorg`](https://github.com/gabrie30/ghorg)
 - [Gitslave (aka `gits`)](https://gitslave.sourceforge.net/)
 - [git-workspace](https://github.com/orf/git-workspace)
 - [git-poly](https://luke_titley.gitlab.io/git-poly/)
