# grit

`grit` is a tool for cloning, updating and organising git repositories. It's like `go get` for git.

`grit` differs from other similar tools in that it:
  * uses a minimalistic config, so you can use it immediately with sensible defaults
  * relies on the git CLI for all git operations, so all config that applies to git is respected
  * uses concurrency wherever possible, so it's fast
  * supports nested trees of git repositories, like you find in Gitlab Groups


## How it works

`grit` organises your git repositories in your workspace directory in a tree structure that mirrors the URL structure of the remote git repository.

For example, if you have a git repository with the URL github.com/my-org/my-repo, then `grit` will clone it into `$GRIT_WORKSPACE/github.com/my-org/my-repo`.

- `grit get` - clones a git repository
- `grit sync` - synchronises and updates all git repositories in the workspace
- `grit list` - lists all git repositories in the workspace
- `grit status` - shows the status of all git repositories in the workspace


Features
 - `grit get` repos from GitHub or Gitlab by using the project URL without the proceeding https:// or git@
 - `@latest` = the tag with the highest semver version

A useful alias for changing directory to a repo is:
```
alias gritcd="cd \$(grit list --full-path | fzf) && pwd"
```

## Example use

```shell
export GRIT_WORKSPACE=~/Developer/src          # Set the current directory as the grit workspace. The git workspace is a directory
                                               # that mirrors the URL structure remote git repositories.
grit get github.com/<ORG-NAME>/<PROJECT-NAME>  # Clone a repo from GitHub or Gitlab using the project URL. The repo will be cloned
                                               #   into $GRIT_WORKSPACE/github.com/<ORG-NAME>/<PROJECT-NAME>
grit get -r github.com/<ORG-NAME>              # Clone all remote repos from a GitHub or Gitlab org in parallel.
grit sync --archive github.com/<ORG-NAME>      #   --archive:    Move local repos that are not found remotely to `$GRIT_WORKSPACE/.archive/`
grit sync --pristine github.com/<ORG-NAME>     #   --pristine:   Restore local repos to pristine state with stash, reset, clean
grit get --pristine --except-for my-repo       #   --except-for: Except for repos that match the 'my-repo' glob
grit list                                      # List all local repos in the workspace
grit status                                    # Show the status of all local repos that have changes
```

## Configuration

- `GRIT_WORKSPACE` can be set to a directory
- `GITLAB_HOSTS` can be set to a comma separated list of custom Gitlab hosts

### Authentication using `.netrc`

In order to use the `grit sync` command, you'll need to use the Github or Gitlab API. You can set up authentication for GitHub and Gitlab using your `.netrc` file.

For example:
```
machine github.com
  login PRIVATE-TOKEN
  password <YOUR-GITHUB-PERSONAL-ACCESS-TOKEN>

machine api.github.com
  login PRIVATE-TOKEN
  password <YOUR-GITHUB-PERSONAL-ACCESS-TOKEN>

machine gitlab.com
  login PRIVATE-TOKEN
  password <YOUR-GITLAB-PERSONAL-ACCESS-TOKEN>
```


## Prior art
 - https://gerrit.googlesource.com/git-repo
 - https://github.com/gabrie30/ghorg
 - https://gitslave.sourceforge.net/
 - https://github.com/orf/git-workspace
 - https://luke_titley.gitlab.io/git-poly/
 - https://github.com/grdl/git-get
 - https://github.com/fboender/multi-git-status
 - https://github.com/x-motemen/ghq

