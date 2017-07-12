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

Download the latest version of `regis-companion`, replacing `${FILENAME}` with the desired output file path, and `${VERSION}` with the latest available version (see the [Releases][releases] tab in Github):

```
# download e.g. using curl
$ curl -o ${FILENAME} https://github.com/harfangapps/regis-companion/releases/download/v${VERSION}/regis-companion_${VERSION}_macOS-64bit.tar.gz

# then extract the binary
$ tar -xzf ${FILENAME}

# optionnally, copy it to some location in your $PATH (recommended)
```

## Running As A launchd Service (RECOMMENDED)

If you installed using Homebrew, then running `regis-companion` as a `launchd` service is simple:

```
$ brew services start regis-companion
```

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
[releases]: https://github.com/harfangapps/regis-companion/releases
[bsd]: http://opensource.org/licenses/BSD-3-Clause

