# gitorg

`gitorg` is a cli tool for organising and syncing large numbers of git repositories in a consistent and fast way.


## Why use `gitorg`?

`gitorg` streamlines cloning repos to a consistent location, and keeps them up-to-date with their remote. It's useful for developers who work with many git repositories, and especially for those who work with many repositories from multiple GitHub or GitLab orgs.

`gitorg`'s goals:
  * sensible defaults: so you can use it immediately without any special setup or config
  * minimal config: relies on the git CLI for all git operations, so all config that applies to git is respected
  * fast: uses concurrency wherever possible
  * small: a focussed feature-set, so it's easy to understand and use
  * make it just work: interoperable with popular git hosting services GitHub and GitLab, support Gitlab Groups and nested repositories


## How it works

`gitorg` organises your git repositories in your workspace directory in a tree structure that mirrors the URL structure of the remote git repository. For example, if you have a git repository with the URL `https://github.com/my-org/my-repo`, then `gitorg` will clone it into `$GITORG_WORKSPACE/github.com/my-org/my-repo`.

There are three commands.
- `gitorg get REPO_URL@COMMIT` will clone a repository using the repo's web URL.
- `gitorg sync ORG_URL` will recursively clone or pull all repositories using the GitHub or GitLab org, user or group URL.
- `gitorg list` will list all git repositories in the workspace.

Note that `gitorg` uses:
 - `origin` as the default remote
 - `https` as the git transport. To use SSH instead, override the URL in your `.gitconfig` (see example below)


## Example use

```shell
export GITORG_WORKSPACE=~/Developer/src   # Set the gitorg workspace. The gitorg workspace is a directory that mirrors
                                          # the remote repository URL structure.
gitorg get github.com/my-org/my-project   # Clone a repo into $GITORG_WORKSPACE/github.com/my-org/my-project
gitorg sync github.com/my-org             # Clone all remote repos from the remote org in parallel
gitorg list                               # List all local repos in the workspace
```


## Configuration

- `GITORG_WORKSPACE` can be set to a directory where you want to store your git repositories. By default it will use ~/gitorg
- `GITLAB_HOSTS` can be set to a comma separated list of custom GitLab hosts
- A `$GITORG_WORKSPACE/.gitorgignore` file can be used to ignore certain repos when using `gitorg sync`. This file uses the same syntax as `.gitignore` files and also applies to remote repos.

### Authentication

In order to use the `gitorg sync` command, you'll need to use the GitHub or GitLab API. You can set up authentication for GitHub and GitLab using your `.netrc` file.

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

Using shell autocompletion is useful, install it in your shell with `gitorg completion`

If you wish to use SSH transport instead of HTTPS, you can override the URL in your `.gitconfig` file. For example:
```ini
[url "git@github.com:"]
	insteadOf = https://github.com/
[url "git@gitlab.com:"]
	insteadOf = https://gitlab.com/
```


## TODO: wanted features
 - ~--except-for or ignorefile~
 - list --status --tree
 - --tidy: - find directories not part of remote
 - graceful shutdown - "index.lock exists Another git process seems to be running in this repository"
 - oauth2 authentication
 - `@latest` = the tag with the highest semver version
 - don't include skipped updates in stats
 - Ctrl-C during sync should display errors. Or just print errors in realtime


## Prior art and inspiration
 - https://gerrit.googlesource.com/git-repo
 - https://github.com/gabrie30/ghorg
 - https://github.com/x-motemen/ghq
 - https://gitslave.sourceforge.net/
 - https://github.com/orf/git-workspace
 - https://luke_titley.gitlab.io/git-poly/
 - https://github.com/grdl/git-get
 - https://github.com/fboender/multi-git-status
