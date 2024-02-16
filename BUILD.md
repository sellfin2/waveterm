# Build Instructions for Wave Terminal

These instructions are for setting up the build on MacOS.
If you're developing on Linux please use the [Linux Build Instructions](./build-linux.md).

## Running the Development Version of Wave

If you install the production version of Wave, you'll see a semi-transparent gray sidebar, and the data for Wave is stored in the directory ~/.waveterm. The development version has a blue sidebar and stores its data in ~/.waveterm-dev. This allows the production and development versions to be run simultaneously with no conflicts. If the dev database is corrupted by development bugs, or the schema changes in development it will not affect the production copy.

## Prereqs and Tools

Download and install Go (must be at least go 1.18):

```
brew install go
```

Download and install ScriptHaus (to run the build commands):

```
brew tap scripthaus-dev/scripthaus
brew install scripthaus
```

You also need a relatively modern nodejs with npm and yarn installed.

-   Node can be installed from [https://nodejs.org](https://nodejs.org).
-   npm can install yarn using:

```
npm install -g yarn
```

## Clone the Repo

```
git clone git@github.com:wavetermdev/waveterm.git
```

## Building WaveShell / WaveSrv

```
scripthaus run build-backend
```

This builds the Golang backends for Wave. The binaries will put in waveshell/bin and wavesrv/bin respectively. If you're working on a new plugin or other pure frontend changes to Wave, you won't need to rebuild these unless you pull new code from the Wave Repository.

## One-Time Setup

Install modules (we use yarn):

```
yarn
```

Electron also requires specific builds of node_modules to work (because Electron embeds a specific node.js version that might not match your development node.js version). We use a special electron command to cross-compile those modules:

```
scripthaus run electron-rebuild
```

## Running WebPack

We use webpack to build both the React and Electron App Wrapper code. They are both run together using:

```
scripthaus run webpack-watch
```

## Running the WaveTerm Dev Client

Now that webpack is running (and watching for file changes) we can finally run the WaveTerm Dev Client! To start the client run:

```
scripthaus run electron
```

To kill the client, either exit the Electron App normally or just Ctrl-C the `scripthaus run electron` command.

Because we're running webpack in watch mode, any changes you make to the typescript will be automatically picked up by the client after a refresh. Note that I've disabled hot-reloading in the webpack config, so to pick up new changes you'll have to manually refresh the WaveTerm Client window. To do that use "Option-R" (Command-R is used internally by WaveTerm and will not force a refresh).

## Debugging the Dev Client

You can use the regular Chrome DevTools to debug the frontend application. You can open the DevTools using the keyboard shortcut `Cmd-Option-I`.
