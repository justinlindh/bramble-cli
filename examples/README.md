# Bramble CLI Examples

Practical usage patterns for common Bramble CLI workflows.

## Examples

| Script | Description |
|--------|-------------|
| [01-connect.sh](01-connect.sh) | BLE vs WiFi vs serial connection |
| [02-send-receive.sh](02-send-receive.sh) | Basic send and receive messages |
| [03-channels.sh](03-channels.sh) | Channel operations (list, add, remove, default) |
| [04-location.sh](04-location.sh) | Location sharing and peer location status |
| [05-monitor.sh](05-monitor.sh) | Monitor and debug output patterns |

## Quick Start

```bash
# Auto-detect USB serial node
bramble status

# Confirm you can see peers
bramble peers
```

All examples assume `bramble` is installed and on your `$PATH`.  
Each script is standalone — source it or run it directly: `bash examples/01-connect.sh`
