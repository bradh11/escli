<p align="center">
  <strong>escli</strong>
</p>
<p align="center">
  <em>Read your Elasticsearch data from the terminal. Fast to set up, safe by default, ready for AI agents.</em>
</p>
<p align="center">
  <a href="https://github.com/bradh11/escli/actions/workflows/ci.yml"><img src="https://github.com/bradh11/escli/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI"></a>
  <a href="https://github.com/bradh11/escli/releases"><img src="https://img.shields.io/github/v/release/bradh11/escli" alt="Latest release"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/bradh11/escli" alt="Go version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/bradh11/escli" alt="License"></a>
</p>

---

**Source code**: <a href="https://github.com/bradh11/escli">https://github.com/bradh11/escli</a>

---

escli is a read-only command-line client for Elasticsearch, built for people
**and** for AI agents.

The key features are:

* **Read-only by default**: every command only reads data. The `api` command
  refuses any request that could write, unless you explicitly pass
  `--allow-write`. 🔒
* **Secure**: your API key lives in your system keychain, never in a plain
  text file (unless there is no keychain at all), and never in the binary.
* **Easy**: one command, `escli init`, sets everything up. Paste the Kibana
  URL straight from your browser and escli works out the rest.
* **JSON everywhere**: every command prints JSON to stdout and errors to
  stderr, so you can pipe the output into `jq`, scripts and other tools.
* **Agent-ready**: the commands, flags and output are designed for an AI agent
  to call. A ready-made [skill](skills/escli) teaches the agent how. 🤖
* **Zero dependencies**: one static binary for macOS, Linux and Windows.
  Download it and run it.

## Requirements

An Elasticsearch cluster (self-managed, Elastic Cloud or Serverless) and an
API key for it. You will create the key in a minute.

That's it. You don't need Python, Node or anything else installed.

## Installation

### Download a release

Download the archive for your platform from the
[releases page](https://github.com/bradh11/escli/releases), extract it and
put `escli` on your `PATH`:

```console
$ tar -xzf escli_*_darwin_arm64.tar.gz
$ sudo mv escli /usr/local/bin/
```

> [!TIP]
> On macOS, your browser quarantines downloaded files, and Gatekeeper blocks
> the unsigned binary. Remove the quarantine flag once:
>
> ```console
> $ xattr -d com.apple.quarantine /usr/local/bin/escli
> ```

> [!NOTE]
> Each release has a `checksums.txt` file. You can verify your download with
> `shasum -a 256 -c checksums.txt --ignore-missing`.

### With Go

If you have Go installed, you can also install it with:

```console
$ go install github.com/bradh11/escli/cmd/escli@latest
```

## Example

### Create an API key

First, create an API key for yourself. Each person should have their own key,
so that access can be revoked per person.

In Kibana, open **Stack Management > Security > API keys** and select
**Create API key**:

1. Give it a name that says whose key it is, for example `pat-laptop`.
2. Set an expiry date, for example 90 days.
3. Turn on **Control security privileges** and give the key read access only:

   ```json
   {
     "escli-read": {
       "cluster": ["monitor"],
       "indices": [
         {
           "names": ["*"],
           "privileges": ["read", "view_index_metadata"]
         }
       ]
     }
   }
   ```

4. Copy the key. Kibana shows it only once. Any of the formats Kibana offers
   works.

> [!TIP]
> Replace `"*"` with only the index patterns you need. The less a key can
> read, the less it can leak.

### Set it up

Now run `escli init`:

```console
$ escli init
Elasticsearch URL, Kibana URL or Cloud ID: https://my-project.kb.us-east-2.aws.elastic.cloud
Note: escli converted the Kibana address to the Elasticsearch address.
Elasticsearch address: https://my-project.es.us-east-2.aws.elastic.cloud

Paste your API key. To create one, open Kibana > Stack Management > Security > API keys:
  https://my-project.kb.us-east-2.aws.elastic.cloud/app/management/security/api_keys
API key (input hidden):
Signed in as pat@example.com (API key "pat-laptop").
Saved profile "default" in /Users/pat/.config/escli/config.json and the API key in the system keychain.
```

Notice that you can paste the **Kibana** URL right from your browser. escli
converts it to the Elasticsearch address for you. ✨

### Explore it

See what's in your cluster:

```console
$ escli indices
```

You will see your indices, aliases and data streams as JSON. Add a pattern
such as `'deals*'` to narrow it down, or `--stats` for document counts and
sizes.

Then look at the fields of an index:

```console
$ escli mapping deals --flat
```

### Query it

Now read some data. Search with a Lucene query:

```console
$ escli search deals -q 'stage:closedwon AND amount:>10000' --size 5 --fields name,amount
```

Or use ES|QL to filter, aggregate and sort:

```console
$ escli esql 'FROM deals | STATS total = SUM(amount) BY stage | SORT total DESC'
```

### Recap

In summary, you:

* Created an API key that can only **read**.
* Ran `escli init` **once**, and your key went into the system keychain.
* Explored the cluster with `indices` and `mapping`.
* Read data with `search` and `esql`, and got **JSON** back.

And all of that with a single binary. 🚀

## Usage

Here are the commands you will use most:

```sh
escli indices                       # indices, aliases and data streams
escli indices 'deals*' --stats      # with document counts and sizes
escli mapping deals --flat          # field names and types
escli search deals -q 'stage:closedwon AND amount:>10000' --size 5 --fields name,amount
escli search deals --sort amount:desc --size 3
escli count deals -q 'stage:closedwon'
escli get deals 12345
escli esql 'FROM deals | STATS total = SUM(amount) BY stage | SORT total DESC'
escli esql 'FROM deals | KEEP name, amount | LIMIT 20' --format csv
escli api GET /_cluster/health
escli auth status                   # who the key signs in as
```

### `search`

`search` prints the total and each document's `_source`, with its `_index`
and `_id`. Use `--raw` to get the complete Elasticsearch response instead.

`--body` takes a full search request as JSON, as `@file`, or as `-` to read
from stdin. The other flags override the matching keys of the body.

### `esql`

`esql` prints one JSON object per row. Use `--format csv|tsv|txt` for a
table, or `--raw` for the columns and values arrays.

> [!TIP]
> Add `--compact` to any command for single-line JSON.

## Configuration

### Where things are stored

| What | Where |
|---|---|
| Cluster address | `~/.config/escli/config.json` (`%AppData%\escli` on Windows) |
| API key | macOS Keychain, Windows Credential Manager or Linux Secret Service |

> [!NOTE]
> On a Linux machine without a Secret Service, such as a server or a
> container, escli stores the key in `~/.config/escli/credentials.json` with
> mode `0600`, and tells you so.

When your key expires, just run `escli init` again.

### More than one cluster

Do you have a staging cluster too? Use profiles:

```sh
escli init --profile staging   # set up a second profile; it becomes the current one
escli config list              # show the profiles
escli config use default       # change the current profile
escli --profile staging indices
```

### CI and scripts

In CI, set `ESCLI_URL` and `ESCLI_API_KEY` instead of running `escli init`.
They override the profile.

To create a profile without any prompts:

```sh
echo "$API_KEY" | escli init --url "$ES_URL" --api-key-stdin
```

## Using escli with AI agents

escli is designed for agents to call. 🤖

First, run `escli init` **yourself**. The agent should never see or handle
your API key. After that, the agent can call escli without any prompts.

The skill in [`skills/escli`](skills/escli) teaches an agent how to explore a
cluster with escli: check the setup, look at the indices and fields first, and
only then query. For Claude Code, copy it to `~/.claude/skills/escli/`.

> [!TIP]
> Give the agent a key with access only to the indices it needs. Read-only
> commands are a good start, and a narrow key is even better.

## Authentication

escli signs in with an Elasticsearch API key.

Signing in with Google (OAuth) is not supported yet. On Elastic Cloud, Kibana
signs people in through Elastic Cloud SSO, and Elasticsearch accepts a Google
token only when an administrator sets up a JWT realm that trusts Google.
Elastic Cloud Serverless projects do not offer that.

If your cluster has such a realm and you would like an `escli login` command,
open an issue. The `es` package only needs a different `Authorization`
header.

## Contributing

Bug reports, ideas and pull requests are all very welcome. 🎉

Read [CONTRIBUTING.md](CONTRIBUTING.md) to get started. To report a security
issue, follow [SECURITY.md](SECURITY.md) instead of opening a public issue.

## License

This project is licensed under the terms of the [MIT license](LICENSE).
