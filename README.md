# orgit

`orgit` is a cli tool for organising and syncing git repositories in a consistent and fast way.


## Why use `orgit`?

`orgit` streamlines cloning git repos to a consistent location, and keeping them up-to-date. It's useful for developers who work with a large number of repositories within GitHub or GitLab orgs.

`orgit`'s goals:
  * sensible defaults: you can use it immediately without any special setup or config
  * minimal config: it relies on the git CLI for all git operations, so all config that applies to git is respected
  * fast: it uses concurrency wherever possible to parallise API and git operations
  * small: a focussed feature-set, so it's easy to understand and use
  * it just works: interoperable with popular git hosting services


## How it works

`orgit` organises your git repositories in your workspace directory in a tree structure that mirrors the URL structure of the remote git repository. For example, if you have a git repository with the URL `https://github.com/my-org/my-repo`, then `orgit` will clone it into `$ORGIT_WORKSPACE/github.com/my-org/my-repo`.

There are three commands.
- `orgit get REPO_URL@COMMIT` will clone a repository using the repo's HTTP URL.
- `orgit sync ORG_URL` will recursively clone or pull all repositories using the GitHub or GitLab org, user or group URL.
- `orgit list` will list all git repositories in the workspace.

Note that `orgit` always uses:
 - `origin` as the default remote
 - `https` as the git transport. To use SSH instead, override the URL in your `.gitconfig` (see example below)

## Installing

Either
 1. download the latest release from the [releases page](https://github.com/mtibben/orgit/releases), or
 2. install using your go toolchain: `go install github.com/mtibben/orgit@latest`.


## Example use

```shell
export ORGIT_WORKSPACE=~/Developer/src   # Set the orgit workspace. The orgit workspace is a directory that mirrors
                                         # the remote repository URL structure.
orgit get github.com/my-org/my-project   # Clone a repo into $ORGIT_WORKSPACE/github.com/my-org/my-project
orgit sync github.com/my-org             # Clone all repos from the remote org in parallel
orgit list                               # List all local repos in the workspace
```


## Configuration

- `ORGIT_WORKSPACE` can be set to a directory where you want to store your git repositories. By default it will use `~/orgit`
- `GITLAB_HOSTS` can be set to a comma separated list of custom GitLab hosts
- A `$ORGIT_WORKSPACE/.orgitignore` file can be used to ignore certain repos when using `orgit sync`. This file uses the same syntax as `.gitignore` files and also applies to remote repos.

### Authentication

In order to use the `orgit sync` command, you'll need to use the GitHub or GitLab API. You can set up authentication for GitHub and GitLab using your `.netrc` file.

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


## Tips

A useful shell alias for changing directory to a repo using `fzf`
```shell
alias gcd="cd \$(orgit list --full-path | fzf) && pwd"
```

Using shell autocompletion is useful, install it in your shell with `orgit completion`

If you wish to use SSH transport instead of HTTPS, you can override the URL in your `.gitconfig` file. For example:
```ini
[url "git@github.com:"]
	insteadOf = https://github.com/
[url "git@gitlab.com:"]
	insteadOf = https://gitlab.com/
```


## TODO wanted features for v1
 - ~ignorefile like .gitignore~ done
 - ~Ctrl-C to cancel sync~ done
 - ~releases~ done
 - ~graceful shutdown~ done
 - ~fix error stats on graceful shutdown. Race condition, need to synchronise cancel and progress printer~
 - ~--tidy: - find directories not part of remote~
 - ~handle moved repos~
 - don't include skipped updates in stats
 - list --status --tree
 - oauth2 authentication
 - `@latest` = the tag with the highest semver version
 - better tests

## Prior art and inspiration
 - https://gerrit.googlesource.com/git-repo
 - https://github.com/gabrie30/ghorg
 - https://github.com/x-motemen/ghq
 - https://gitslave.sourceforge.net/
 - https://github.com/orf/git-workspace
 - https://luke_titley.gitlab.io/git-poly/
 - https://github.com/grdl/git-get
 - https://github.com/fboender/multi-git-status
