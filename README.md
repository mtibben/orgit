# gitorg

`gitorg` is a tool for cloning and organising git repositories. It's like `go get` for git.

## Why use `gitorg`?

`gitorg` is useful if you need to manage a large number of git repositories. It's especially useful if you have a lot of git repositories that are organised in a tree structure, like you find in Gitlab Groups.

`gitorg` differs from other similar tools in that it:
  * uses sensible defaults, so you can use it immediately without any special setup or config
  * relies on the git CLI for all git operations, so all config that applies to git is respected
  * uses concurrency wherever possible, so it's fast
  * supports nested trees of git repositories, so it supports Gitlab Groups
  * has a small, focussed feature-set, so it's easy to understand and use

This makes `gitorg` a single, simple replacement for `git clone`, `ghorg` and `ghq`.

## How it works

`gitorg` organises your git repositories in your workspace directory in a tree structure that mirrors the URL structure of the remote git repository. For example, if you have a git repository with the URL github.com/my-org/my-repo, then `gitorg` will clone it into `$GITORG_WORKSPACE/github.com/my-org/my-repo`.

There are three commands.
- `gitorg get PROJECT_URL` will clone a single git repository.
- `gitorg sync ORG_URL` will recursively clone all git repositories in a GitHub organisation or Gitlab group.
- `gitorg list` will list all git repositories in the workspace.

## Example use

```shell
export GITORG_WORKSPACE=~/Developer/src   # Set the gitorg workspace. The git workspace is a directory that mirrors
                                          # the remote git repository's URL structure.
gitorg get github.com/my-org/my-project   # Clone a repo into $GITORG_WORKSPACE/github.com/my-org/my-project
gitorg sync github.com/my-org             # Clone all remote repos from the remote org in parallel
gitorg list                               # List all local repos in the workspace
```

## Configuration

- `GITORG_WORKSPACE` can be set to a directory where you want to store your git repositories. By default it will use ~/gitorg
- `GITLAB_HOSTS` can be set to a comma separated list of custom Gitlab hosts

### Authentication

In order to use the `gitorg sync` command, you'll need to use the Github or Gitlab API. You can set up authentication for GitHub and Gitlab using your `.netrc` file.

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
alias gcd="cd \$(gitorg list --full-path | fzf) && pwd"
```

You can install autocompletion in your shell by running `gitorg completion`. This will install a completion script for bash, zsh, fish, and powershell.


## TODO: wanted features
 - status
 - "tidy" - find directories not part of remote
 - graceful shutdown - "index.lock exists Another git process seems to be running in this repository"
 - Oauth2 authentication
 - except-for repos - ignorefile?
 - `@latest` = the tag with the highest semver version
 - don't include skipped updates in stats

## Prior art and inspiration
 - https://gerrit.googlesource.com/git-repo
 - https://github.com/gabrie30/ghorg
 - https://github.com/x-motemen/ghq
 - https://gitslave.sourceforge.net/
 - https://github.com/orf/git-workspace
 - https://luke_titley.gitlab.io/git-poly/
 - https://github.com/grdl/git-get
 - https://github.com/fboender/multi-git-status
