# Regis Companion

Regis companion is a command-line tool that provides advanced features for the [Regis Mac App](regis) by [Harfang Apps](harfang).

## Installation Instructions

The Regis companion can be installed via [Homebrew][brew] or by downloading a pre-built binary from Github. It is recommended to use Homebrew for a simpler update flow.

It is also recommended to register `regis-companion` as a `launchd` service, see [Running As A launchd Service (RECOMMENDED)][#running-as-a-launchd-service-recommended].

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

### Via Homebrew (RECOMMENDED)

If you installed using Homebrew, then running `regis-companion` as a `launchd` service is simple:

```
$ brew services start regis-companion
```

This makes sure `regis-companion` is always running in the background, waiting for connections from Regis. Note that by default, `regis-companion` only accepts connections from the loopback interface (localhost). It is recommended to leave it that way.

### Manually

If you installed manually, either using pre-built binaries or from source, you can generate a skeleton `launchd` plist file by running:

```
# redirect the output to a file
$ regis-companion --generate-launchd-plist > com.harfangapps.regis-companion.plist
```

You should then review and edit as required the generated plist file, and move it to the `launchd` directory so that it can be registered as a service.

```
$ mv com.harfangapps.regis-companion.plist ~/Library/LaunchAgents/
```

You can then enable or disable the service using `launchctl`:

```
$ launchctl enable gui/${UID}/com.harfangapps.regis-companion
$ launchctl bootstrap gui/${UID} ~/Library/LaunchAgents/com.harfangapps.regis-companion.plist
```

Refer to `launchctl` documentation for details (`$ man launchctl`).

## Features

TODO: or usage?

This companion command supports automatic SSH tunneling for Regis so that it can connect to remote hosts otherwise not available from the local computer.

TODO: find right phrasing, it makes otherwise impossible things possible, it is way more than just a transparent `ssh -N -L`, like automatically following Redis Cluster redirections when the hosts are on a VPN, supports the SWITCHTO built-in command, Sentinels, etc.

## Preparing a Release

The following steps are automated via the Makefile, so generally preparing a release requires:

1. Add version tag to the current git commit, push the tag.
2. Run the following make commands:
    ```
    $ make release
    # read output, some manual steps required

    $ make brew
    # read output, copy the SHA256 to the homebrew recipe and update the version in the URL
    ```

For reference, the detailed steps are:

* [ ] Add version tag to the current git commit
    `$ git tag vM.m.p`
* [ ] Build the binary
    `$ ./misc/scripts/build.sh`
* [ ] Run a test
    `$ ./bin/regis-companion -version`
* [ ] Create the archive for the binary
    `$ tar -czf ./bin/regis-companion_${VERSION}_macOS-64bit.tar.gz ./bin/regis-companion`
* [ ] Upload the binary to the Github Release
* [ ] Update the harfangapps/homebrew-harfangapps tap for the new version

## License

The [BSD 3-Clause license][bsd].

[regis]: https://www.harfangapps.com/regis/
[harfang]: https://www.harfangapps.com/
[brew]: https://brew.sh/
[releases]: https://github.com/harfangapps/regis-companion/releases
[bsd]: http://opensource.org/licenses/BSD-3-Clause

