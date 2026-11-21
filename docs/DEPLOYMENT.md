# Ubuntu Server, CasaOS, and Pi-hole cutover

This deployment uses one container containing:

- DNS over UDP and TCP
- The HTTP API
- The embedded React dashboard
- SQLite at `/data/sinkhole.db`
- Background blocklist refreshes

The target is an Ubuntu Server host managed by CasaOS. Pi-hole can stay installed and running during initial testing.

## Before starting

1. Reserve a static IP for the server in your router's DHCP settings.
2. Confirm the server architecture with `uname -m`.
3. Confirm the repository is on the server or clone it there.
4. Identify your current Pi-hole container and its ports:

```bash
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}'
```

Do not delete the Pi-hole container, its configuration, or its volumes. They are the rollback path.

## Build the image

From the repository root:

```bash
docker build -t network-wide-dns-sinkhole:local .
```

CasaOS's custom-app installer may not build a Dockerfile itself. Building the image first ensures CasaOS only has to start it.

## Test beside Pi-hole

Keep Pi-hole on port 53 and expose this instance on alternate host port `1053`.

Create `.env` in the repository directory:

```dotenv
SINKHOLE_DNS_PORT=1053
SINKHOLE_WEB_PORT=8081
SINKHOLE_IMAGE=network-wide-dns-sinkhole:local
SINKHOLE_UPSTREAMS=1.1.1.1:53,1.0.0.1:53
```

Then start:

```bash
docker compose up -d
docker compose ps
curl --fail http://SERVER_IP:8081/api/health
```

### Verify forwarding and blocking

```bash
dig @SERVER_IP -p 1053 example.com
dig +tcp @SERVER_IP -p 1053 example.com
```

Add a deterministic test rule instead of relying on a public list's current contents:

```bash
curl -X POST http://SERVER_IP:8081/api/rules \
  -H 'Content-Type: application/json' \
  -d '{"domain":"blocked.test","type":"deny","note":"pre-cutover test"}'

dig @SERVER_IP -p 1053 blocked.test
dig +tcp @SERVER_IP -p 1053 blocked.test
```

Both blocked queries should return `NXDOMAIN`. Open `http://SERVER_IP:8081` and verify that all four requests appear in the Query Log with the expected decisions.

The default StevenBlack source begins downloading on first start. Check **Blocklists** until **Domains** is nonzero. A transient download failure is recorded on the list and does not crash DNS service.

### Verify source addresses

Test from two physical LAN devices. The Query Log should show their real addresses rather than a Docker gateway.

If clients appear as one Docker bridge address, verify that the publishing proxy or a prior container is not masking them. If necessary, run the service with host networking: remove the `ports:` mapping from `compose.yaml`, add `network_mode: host`, and retain the container listeners on ports 53 and 8080. CasaOS integration may be reduced in host-network mode.

## CasaOS custom app

1. Build the image on the server.
2. In CasaOS, open **App Store → Install a custom app**.
3. Import `compose.yaml` from this repository.
4. Set `SINKHOLE_DNS_PORT` and `SINKHOLE_WEB_PORT` to free host ports for the side-by-side test.
5. Keep the `sinkhole-data` volume. It contains all rules, blocklists, settings, and query history.
6. Start the app and open the dashboard from CasaOS or `http://SERVER_IP:PORT`.

For strict backup access, bind-mount `/DATA/AppData/sinkhole/data:/data` instead of the named volume. If you do, make the directory writable by UID/GID `10001`:

```bash
sudo mkdir -p /DATA/AppData/sinkhole/data
sudo chown -R 10001:10001 /DATA/AppData/sinkhole/data
```

## Cutover from Pi-hole

Perform these steps only after forwarding, blocking, persistence, and source-address logging pass on port 1053.

1. Note the router's current DNS setting so it can be restored.
2. Stop Pi-hole and leave its data intact:

```bash
docker update --restart=no PI_HOLE_CONTAINER
docker stop PI_HOLE_CONTAINER
```

If Pi-hole is installed natively, stop its existing service without uninstalling it.

3. Confirm that nothing else owns DNS port 53:

```bash
sudo ss -lntup | grep ':53 '
```

4. Change `.env` to use the standard ports:

```dotenv
SINKHOLE_DNS_PORT=53
SINKHOLE_WEB_PORT=8080
```

5. Recreate this container and verify direct queries:

```bash
docker compose up -d
dig @SERVER_IP example.com
dig +tcp @SERVER_IP example.com
dig @SERVER_IP blocked.test
```

6. Configure one client manually to use `SERVER_IP` as its only DNS server. Test normal browsing, blocking, and the Query Log.
7. Change the router's DHCP DNS setting to `SERVER_IP` only after that client works. Avoid advertising a public secondary resolver, because clients may bypass filtering through it.
8. Renew DHCP leases or reconnect devices so they pick up the new resolver.

## Rollback

Restore the previous router/client DNS settings first, then:

```bash
docker compose down
docker start PI_HOLE_CONTAINER
docker update --restart=unless-stopped PI_HOLE_CONTAINER
```

Do not run Pi-hole and this service on the same host port simultaneously.

## Firewall and exposure

Do not port-forward DNS or the dashboard from the internet. An open recursive resolver can be abused for DNS amplification.

For a LAN-only UFW policy, adjust the subnet and ports for your network:

```bash
sudo ufw allow from 192.168.1.0/24 to any port 53 proto udp
sudo ufw allow from 192.168.1.0/24 to any port 53 proto tcp
sudo ufw allow from 192.168.1.0/24 to any port 8080 proto tcp
```

Docker's published ports can bypass parts of UFW, so router rules and Docker `DOCKER-USER` policies must also prevent external access.

## Persistence and upgrades

The named volume survives container recreation and image upgrades. Before upgrading:

```bash
docker run --rm \
  -v sinkhole-data:/data:ro \
  -v "$PWD":/backup \
  alpine tar czf /backup/sinkhole-data-backup.tgz -C /data .
```

Rebuild the image, then recreate the service:

```bash
docker compose up -d --build
```

Migrations run automatically at startup. Never delete `/data` to troubleshoot an upgrade; preserve a backup first.

## Troubleshooting

| Symptom | Check |
|---|---|
| Container restarts | `docker compose logs --tail=100 sinkhole` |
| Port 53 unavailable | `sudo ss -lntup \| grep ':53 '` and `docker ps` |
| DNS resolves but dashboard is empty | Wait for the 500 ms query-log flush and enable polling |
| Blocklist has zero domains | Use **Refresh** and check container logs for the download or parse error |
| All clients show one IP | Review Docker bridge/source NAT and the host-network option above |
| Blocking broke a site | Add an allow rule for the exact domain, then refresh the Query Log |
| Pi-hole web port conflicts | Choose a different `SINKHOLE_WEB_PORT` |
