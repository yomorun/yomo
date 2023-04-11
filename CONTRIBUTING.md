# Contributing to Yomo

Hello, and welcome! Whether you are looking for help, trying to report a bug, thinking about getting involved in the project or about to submit a patch, this document is for you! Its intent is to be both an entry point for newcomers to the community, and a guide/reference for contributors and maintainers.

Consult the Table of Contents below, and jump to the desired section.

## Table of Contents

- [Where to seek for help?](#where-to-seek-for-help)
- [Where to report bugs?](#where-to-report-bugs)
- [Where to submit feature requests?](#where-to-submit-feature-requests)
- [Contributing](#contributing)
  - [Proposing a new plugin](#proposing-a-new-plugin)
  - [Submission Guidelines](#submission-guidelines)
    - [Git branches](#git-branches)
    - [Commit message format](#commit-message-format)
    - [Static linting](#static-linting)
- [Code style](#code-style)

## Where to seek for help?

There are some channels where you can get answers from the community or the maintainers of this project:

- Discord is used by the community

Please avoid opening GitHub issues for general questions or help, as those should be reserved for actual bug reports. The YoMo community is welcoming and more than willing to assist you on those channels!

[Back to TOC](#table-of-contents)

## Where to report bugs?

Feel free to submit an issue on the GitHub repository, we would be grateful to hear about it! Please make sure to respect the GitHub issue template, and include:

1. A summary of the issue

2. A list of steps to reproduce the issue

3. The version of Yomo you encountered the issue with

4. Your Yomo configuration, or the parts that are relevant to your issue

[Back to TOC](#table-of-contents)

## Where to submit feature requests?

You can [submit an issue](https://github.com/yomorun/yomo/issues/new/choose) for feature requests. Please add as much detail as you can when doing so.

[Back to TOC](#table-of-contents)

## Contributing

We welcome contributions of all kinds, you do not need to code to be helpful! All of the following tasks are noble and worthy contributions that you can make without coding:

- Reporting a bug (see the [report bugs](https://github.com/yomorun/yomo/blob/master/CONTRIBUTING.md#where-to-report-bugs) section)

- Helping other members of the community on the support channels

- Fixing a typo in the code

- Fixing a typo in the documentation

- Providing your feedback on the proposed features and designs

- Reviewing Pull Requests

[Back to TOC](#table-of-contents)

### Proposing a new plugin

We **do not** accept new plugins into the core repository. 

If you wish to write a new plugin for your own needs, you should start by reading the [Quick Start](https://github.com/yomorun/yomo/blob/master/README.md#quick-start).

If you already wrote a plugin, and are thinking about making it available to the community, we strongly encourage you to host it on a publicly available repository (like GitHub), and we provide a page to showcase the plugins you write.

[Back to TOC](#table-of-contents)

### Submission Guidelines

Feel free to contribute fixes or minor features, we love to receive Pull Requests! If you are planning to develop a larger feature, come talk to us first!

When contributing, please follow the guidelines provided in this document. They will cover topics such as the different Git branches we use, the commit message format to use or the appropriate code style.

Once you have read them, and you are ready to submit your Pull Request, be sure to verify a few things:

- Your work was based on the appropriate branch (`master` vs. `next`), and you are opening your Pull Request against the appropriate one

- Your commit history is clean: changes are atomic and the git message format was respected

- Rebase your work on top of the base branch (seek help online on how to use `git rebase`; this is important to ensure your commit history is clean and linear)

- The static linting is succeeding: `golangci-lint run`. (see the development documentation for additional details)

- Do not update `CHANGELOG.md` yourself. Your change will be included there in due time if it is accepted, no worries!

If the above guidelines are respected, your Pull Request has all its chances to be considered and will be reviewed by a maintainer.

If you are asked to update your patch by a reviewer, please do so! Remember: **you are responsible for pushing your patch forward**. If you contributed it, you are probably the one in need of it. You must be prepared to apply changes to it if necessary.

If your Pull Request was accepted and fixes a bug, adds functionality, or makes it significantly easier to use or understand Yomo, congratulations! You are now an official contributor to Yomo.

Your change will be included in the subsequent release Changelog, and we will not forget to include your name if you are an external contributor. 

#### Git branches

We work on two branches: `master`, where non-breaking changes land, and `next`, where important features or breaking changes land in-between major releases.

If you have write access to the GitHub repository, please follow the following naming scheme when pushing your branch(es):

- `feat/foo-bar` for new features

- `fix/foo-bar` for bug fixes

- `tests/foo-bar` when the change concerns only the test suite

- `refactor/foo-bar` when refactoring code without any behavior change

- `style/foo-bar` when addressing some style issue

- `docs/foo-bar` for updates to the README.md, this file, or similar documents

[Back to TOC](#table-of-contents)

#### Commit message format

To maintain a healthy Git history, we ask of you that you write your commit messages as follows:

- The tense of your message must be **present**

- Your message must be prefixed by a type, and a scope

- The header of your message should not be longer than 50 characters

- A blank line should be included between the header and the body

Here is a template of what your commit message should look like:

```html
<type>(<scope>): <subject>
<BLANK LINE>
<body>
<BLANK LINE>
<footer>
```

##### Type

The type of your commit indicates what type of change this commit is about. The accepted types are:

- **feat**: A new feature

- **fix**: A bug fix

- **hotfix**: An urgent bug fix during a release process

- **tests**: A change that is purely related to the test suite only (fixing a test, adding a test, improving its reliability, etc...)

- **docs**: Changes to the README.md, this file, or other such documents

- **style**: Changes that do not affect the meaning of the code (white-space trimming, formatting, etc...)

- **perf**: A code change that significantly improves performance

- **refactor**: A code change that neither fixes a bug nor adds a feature, and is too big to be considered just perf

- **chore**: Maintenance changes related to code cleaning that isn't considered part of a refactor, build process updates, dependency bumps, or auxiliary tools and libraries updates (Golangci, Travis-ci, etc...)

##### Scope

The scope is the part of the codebase that is affected by your change. Choosing it is at your discretion, but here are some of the most frequent ones:

- **stream**: Related to Streaming

- **conn**: Communication connection between services (udp, tcp, quic, etc...)

- **plugin**: Support for plug-in development

- **deps**: When updating dependencies (to be used with the chore prefix)

- **conf**: Configuration-related changes (new values, improvements...)

- *: When the change affects too many parts of the codebase at once (this should be rare and avoided)

##### Subject

Your subject should contain a succinct description of the change. It should be written so that:

- It uses the present, imperative tense: "fix typo", and not "fixed" or "fixes"

- It is **not** capitalized: "fix typo", and not "Fix typo"

- It does **not** include a period. 

##### Body

The body of your commit message should contain a detailed description of your changes. Ideally, if the change is significant, you should explain its motivation, the chosen implementation, and justify it.

##### Footer

The footer is the ideal place to link to related material about the change: related GitHub issues, Pull Requests, fixed bug reports, etc..., you can using [keyword](https://docs.github.com/en/github/managing-your-work-on-github/linking-a-pull-request-to-an-issue#linking-a-pull-request-to-an-issue-using-a-keyword) for issues.

##### Examples

Here are a few examples of good commit messages to take inspiration from:

``` 
feat(plugin): support the development of streaming plugins

The user implements the YomoStreamPlugin interface, and with yomo.RunStream function starts a service that supports stream data processing

Close #18
```

[Back to TOC](#table-of-contents)

##### Static linting

The submitted code must be validated by golangci-lint run, reference [Quick Start](https://golangci-lint.run/usage/quick-start/).

To check the code automatically when committing, you can use the [pre-commit](https://pre-commit.com/#quick-start).

- Make sure the [pre-commit](https://pre-commit.com/#installation) is installed

- Make sure the [.pre-commit-config.yaml](https://github.com/yomorun/yomo/blob/master/.pre-commit-config.yaml) file already exists

- Make sure the [.golangci.yml](https://github.com/yomorun/yomo/blob/master/.golangci.yml) file already exists

[Back to TOC](#table-of-contents)

## Code style

To ensure a healthy and consistent codebase, we ask that you respect the style of code used.

Style checking is also controlled by [golangci-lint](https://golangci-lint.run/usage/quick-start/) and [.golangci.yml](https://github.com/yomorun/yomo/blob/master/.golangci.yml), compatible with [effective_go](https://golang.org/doc/effective_go.html)

[Back to TOC](#table-of-contents)








