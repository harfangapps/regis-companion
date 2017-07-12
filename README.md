# Regis Companion

Regis companion is a command-line tool that provides advanced features for the [Regis Mac App](regis) by [Harfang Apps](harfang).

## Installation Instructions

The Regis companion can be installed via [Homebrew][brew] or by downloading a pre-built binary from Github. It is recommended to use Homebrew for a simpler update flow.

### Using Homebrew (RECOMMENDED)

To install Homebrew, follow the [instructions on their home page][brew]. Then install Harfang Apps' Homebrew tap (a `tap` is a third-party repository of Homebrew formulas):

```
$ brew tap harfangapps/harfangapps
```

Then install the `regis-companion` formula:

```
$ brew install regis-companion

# if it conflicts with another formula name, use:
$ brew install harfangapps/harfangapps/regis-companion
```

### Using Pre-Built Binaries

Download the latest version of `regis-companion`:

```
TODO: link
```

TODO: extract information, or is it directly the binary?

## Running As A launchd Service (RECOMMENDED)

TODO: brew services or manually

## Preparing a Release

* [ ] Add version tag to the current git commit
    `$ git tag vM.m.p`
* [ ] Build the binary
    `$ ./misc/scripts/build.sh`
* [ ] Run a test
    `$ ./bin/regis-companion -version`

## License

The [BSD 3-Clause license][bsd].

[regis]: https://www.harfangapps.com/regis/
[harfang]: https://www.harfangapps.com/
[brew]: https://brew.sh/
[bsd]: http://opensource.org/licenses/BSD-3-Clause

