## secret_inject

The start of an app that can be used to get secrets from an external secret manager and use them as exported environment variables.

As of right now, this only supports pulling secrets from doppler, using the `doppler` CLI.

This solution is pretty insecure in its current state. Plus, the project itself is more of a playground for Rust development than anything else...


### build 
```bash
make
```

### run
```bash
./secret_inject --config [path/to/config/file] [--clean] [--debug]
```


### config file
```json
{
  "sources": {
    // the params required to pull secrets from doppler 
    "doppler": {
      "env": "env_name",
      "project": "project_name"
    }
  },
 // storage / cache options
  "storage": {
    // "file" or "keyring"
    // "file" will store the secrets in an insecure file in the system's temp directory
    // "keyring" will store the secrets using the keyring library (macOS keychain, windows credential manager, jwt, etc.)
    "type": "keyring",

    // if "keyring" is used, these are the underlying backends that are allowed to be used as a storage cache
    // here are the available backends: 
	// "secret-service"
	// "keychain"
	// "keyctl"
	// "kwallet"
	// "wincred"
	// "file"
	// "pass"
    "allowed_backends": ["file"],

    // this is the "master" password for the cache, if not provided, the user will be prompted for it regularly
    "password": "secret"
  }
}
```

### bash_profile entry
```bash
OUTPUT=$(secret_inject)
RESULT=$?
if [ $RESULT -eq 0 ]; then
  eval "$OUTPUT"
else
  echo "$OUTPUT" >> /dev/stderr
fi
```

