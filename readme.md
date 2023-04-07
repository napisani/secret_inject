## secret_inject

The start of an app that can be used to get secrets from an external secret manager and use them as exported environment variables.

As of right now, this only supports pulling secrets from doppler, using the `doppler` CLI.

This solution is pretty insecure in its current state. Plus, the project itself is more of a playground for Rust development than anything else...


### install
```bash
cargo build --release
sudo cp target/release/secret_inject /usr/local/bin/
```

### bash_profile entry
```bash
ENV_VARS_SEC_CONFIG_SLUG=workstation_1
OUTPUT=$(secret_inject --project workspace_env_vars --env $ENV_VARS_SEC_CONFIG_SLUG)
RESULT=$?
if [ $RESULT -eq 0 ]; then
  source "$OUTPUT"
else
  echo "$OUTPUT" >> /dev/stderr
fi
```
