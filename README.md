# grit

`grit` is a tool for cloning and organising git repositories. It's like `go get` for git.

## Why use `grit`?

`grit` is useful if you need to manage a lot of git repositories. It's especially useful if you have a lot of git repositories that are organised in a tree structure, like you find in Gitlab Groups.

`grit` differs from other similar tools in that it:
  * uses minimal config, so you can use it immediately without any setup
  * relies on the git CLI for all git operations, so all config that applies to git is respected
  * uses concurrency wherever possible, so it's fast
  * supports nested trees of git repositories, so it supports Gitlab Groups
  * has a small, focussed feature-set, so it's easy to understand and use

## How it works

`grit` organises your git repositories in your workspace directory in a tree structure that mirrors the URL structure of the remote git repository. For example, if you have a git repository with the URL github.com/my-org/my-repo, then `grit` will clone it into `$GRIT_WORKSPACE/github.com/my-org/my-repo`.

There are two key commands. `grit get` will clone a single git repository.
And `grit sync` will recursively clone all git repositories in a GitHub organisation or Gitlab group.

## Example use

```shell
export GRIT_WORKSPACE=~/Developer/src          # Set the current directory as the grit workspace. The git workspace is a directory
                                               # that mirrors the URL structure remote git repositories.
grit get github.com/<ORG-NAME>/<PROJECT-NAME>  # Clone a repo from GitHub or Gitlab using the project URL. The repo will be cloned
                                               #   into $GRIT_WORKSPACE/github.com/<ORG-NAME>/<PROJECT-NAME>
grit sync github.com/<ORG-NAME>              # Clone all remote repos from a GitHub or Gitlab org in parallel.
grit sync --archive github.com/<ORG-NAME>      #   --archive:    Move local repos that are not found remotely to `$GRIT_WORKSPACE/.archive/`
grit sync --pristine github.com/<ORG-NAME>     #   --pristine:   Restore local repos to pristine state with stash, reset, clean
grit get --pristine --except-for my-repo       #   --except-for: Except for repos that match the 'my-repo' glob
grit list                                      # List all local repos in the workspace
grit list --status                             # Show the status of all local repos that have changes
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

## More tips
 - `@latest` = the tag with the highest semver version
 - A useful alias for changing directory to a repo using `fzf`
    ```
    alias gritcd="cd \$(grit list --full-path | fzf) && pwd"
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

