# Media collector

## Getting started

### Compile & install

```bash
go build media_collector.go
sudo cp media_collector /usr/local/bin/
```

### Bilibili

```bash
# login and scan the QR code
./media-collector bilibili login

# download a single video
./media-collector bilibili download single --bvid <BVID>

# download to-view videos
./media-collector bilibili download to-view

# download videos with search
./media-collector bilibili download search <KEYWORD>
```
