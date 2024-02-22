# Init

Sample configuration files for:

```
systemd: lndltc.service
```

## systemd

Add the example `lndltc.service` file to `/etc/systemd/system/` and modify it according to your system and user configuration. Use the following commands to interact with the service:

```bash
# Enable lndltc to automatically start on system boot
systemctl enable lndltc

# Start lndltc
systemctl start lndltc

# Restart lndltc
systemctl restart lndltc

# Stop lndltc
systemctl stop lndltc
```

Systemd will attempt to restart lndltc automatically if it crashes or otherwise stops unexpectedly.
