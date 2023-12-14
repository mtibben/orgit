# gitorg

`gitorg` is a tool for cloning and organising git repositories. It's like `go get` for git.

## Why use `gitorg`?

`gitorg` is useful if you need to manage a large number of git repositories. It's especially useful if you have a lot of git repositories that are organised in a tree structure, like you find in Gitlab Groups.

`gitorg` differs from other similar tools in that it:
  * uses minimal config, so you can use it immediately without any special setup
  * relies on the git CLI for all git operations, so all config and that applies to git is respected
  * uses concurrency wherever possible, so it's fast
  * supports nested trees of git repositories, so it supports Gitlab Groups
  * has a small, focussed feature-set, so it's easy to understand and use

## How it works

`gitorg` organises your git repositories in your workspace directory in a tree structure that mirrors the URL structure of the remote git repository. For example, if you have a git repository with the URL github.com/my-org/my-repo, then `gitorg` will clone it into `$GRIT_WORKSPACE/github.com/my-org/my-repo`.

There are two key commands. `gitorg get` will clone a single git repository.
And `gitorg sync` will recursively clone all git repositories in a GitHub organisation or Gitlab group.

## Example use

```shell
export GRIT_WORKSPACE=~/Developer/src          # Set the current directory as the gitorg workspace. The git workspace is a directory
                                               # that mirrors the URL structure remote git repositories.
gitorg get github.com/<ORG-NAME>/<PROJECT-NAME>  # Clone a repo from GitHub or Gitlab using the project URL. The repo will be cloned
                                               #   into $GRIT_WORKSPACE/github.com/<ORG-NAME>/<PROJECT-NAME>
gitorg sync github.com/<ORG-NAME>                # Clone all remote repos from a GitHub or Gitlab org in parallel.
gitorg sync --archive github.com/<ORG-NAME>      #   --archive: Move local repos that are not found remotely to `$GRIT_WORKSPACE/.archive/`
gitorg sync --update github.com/<ORG-NAME>       #   --update: Stash uncommitted changes and switch to origin HEAD
gitorg list                                      # List all local repos in the workspace
```

## Configuration

- `GRIT_WORKSPACE` can be set to a directory
- `GITLAB_HOSTS` can be set to a comma separated list of custom Gitlab hosts

### Authentication using `.netrc`

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

## More tips
 - `@latest` = the tag with the highest semver version
 - A useful alias for changing directory to a repo using `fzf`
    ```
    alias gitorgcd="cd \$(gitorg list --full-path | fzf) && pwd"
    ```

## Wanted features
 - status
 - "tidy" - find directories not part of remote
 - graceful shutdown - index.lock exists Another git process seems to be running in this repository
 - Oauth2 authentication
 - except-for repos - ignorefile?

## Prior art
 - https://gerrit.googlesource.com/git-repo
 - https://github.com/gabrie30/ghorg
 - https://gitslave.sourceforge.net/
 - https://github.com/orf/git-workspace
 - https://luke_titley.gitlab.io/git-poly/
 - https://github.com/grdl/git-get
 - https://github.com/fboender/multi-git-status
 - https://github.com/x-motemen/ghq

