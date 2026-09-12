"""Active text infrastructure and real isolated reverse-proxy checks."""

import ast
import importlib.util
import hashlib
import io
import json
from pathlib import Path
import secrets
import tarfile
import time
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
RETIRED_KEYS = {"KNOWOFF_STORAGE_ACCESS_KEY", "KNOWOFF_STORAGE_SECRET_KEY", "KNOWOFF_MEDIA_URL_KEY"}


def snapshot_tools():
    spec = importlib.util.spec_from_file_location("snapshot_infra", ROOT / "infra/compose/snapshot.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class ActiveInfrastructureTests(unittest.TestCase):
    def test_rendered_core_has_only_current_dependencies_and_mounts(self):
        raw = snapshot_tools().CommandRunner(overall=30).run([
            "docker", "compose", "-f", str(ROOT / "infra/compose/docker-compose.yaml"),
            "--profile", "core", "config", "--format", "json"])
        # Never print the expanded environment values from Compose.
        model = json.loads(raw)
        services = model["services"]
        self.assertEqual(set(services), {"postgres", "redis", "migrate", "server", "adminer", "nginx"})
        self.assertEqual(set(model["volumes"]), {"pg_data", "redis_data"})
        self.assertEqual(set(services["server"]["depends_on"]), {"postgres", "redis", "migrate"})
        self.assertEqual(set(services["nginx"]["depends_on"]), {"server", "adminer"})
        for name, service in services.items():
            with self.subTest(service=name):
                self.assertFalse(RETIRED_KEYS.intersection(service.get("environment", {})))
                targets = {v["target"] for v in service.get("volumes", [])}
                self.assertTrue(targets.isdisjoint({"/data/packs", "/data/ingest"}))

    def test_service_templates_do_not_supply_retired_keys(self):
        for name in ("server", "migrate", "seed-admin"):
            with self.subTest(service=name):
                path = ROOT / "infra/compose/config" / name / "environment.env"
                keys = {line.split("=", 1)[0] for line in path.read_text().splitlines()
                        if line and not line.startswith("#") and "=" in line}
                self.assertFalse(keys.intersection(RETIRED_KEYS))
                self.assertTrue({"KNOWOFF_DB_PASSWORD", "KNOWOFF_REDIS_PASSWORD", "KNOWOFF_JWT_KEY"} <= keys)
        self.assertFalse((ROOT / "infra/compose/config/minio/environment.env").exists())

    def test_proxy_has_no_playable_object_backend_or_csp_origin(self):
        files = [ROOT / "nginx/nginx.conf", *sorted((ROOT / "nginx/conf.d").glob("*.conf"))]
        for path in files:
            with self.subTest(file=path.name):
                self.assertNotIn("minio", path.read_text().lower())
                self.assertNotIn("localhost:9000", path.read_text())
        app = (ROOT / "nginx/conf.d/app.knowoff.local.conf").read_text()
        self.assertIn("img-src 'self' data: blob:", app)
        self.assertIn("https://www.gstatic.com", app)

    def test_host_generator_has_only_current_domains_without_touching_hosts(self):
        tree = ast.parse((ROOT / "xops/makefile/hosts_ops.py").read_text())
        domains = next(ast.literal_eval(n.value) for n in tree.body
                       if isinstance(n, ast.Assign) and any(isinstance(t, ast.Name) and t.id == "DOMAINS" for t in n.targets))
        self.assertEqual(set(domains), {"knowoff.local", "app.knowoff.local", "api.knowoff.local",
                                        "admin.knowoff.local", "adminer.knowoff.local"})

    def test_avatar_icons_and_historical_restore_inputs_remain(self):
        for path in ("client/assets/fonts/Baloo2-Variable.ttf", "client/web/favicon.png",
                     "client/web/icons/Icon-192.png", "client/web/icons/Icon-512.png",
                     "content/packs/core-2026.10/manifest.json", "server/internal/avatar/avatar.go"):
            with self.subTest(path=path):
                self.assertGreater((ROOT / path).stat().st_size, 0)
        for path in ("server/Dockerfile", "server/Dockerfile.dev"):
            self.assertIn("libwebp", (ROOT / path).read_text())
        self.assertIn("minio", snapshot_tools().IMAGES)


class IsolatedNginxTests(unittest.TestCase):
    def test_actual_config_and_callback_logs_without_object_store(self):
        ops = snapshot_tools()
        run = ops.CommandRunner(timeout=30, overall=700, max_output=16 * 1024 * 1024)
        token = secrets.token_hex(6)
        label = "knowoff.infra-test=" + token
        name = "knowoff-infra-" + token
        image = name + ":test"
        work = Path(tempfile.mkdtemp(prefix=name + "-", dir="/tmp/agent-runs"))
        network = None
        containers = []
        image_created = False

        def docker(*args, **kw):
            output = io.BytesIO()
            try:
                # Merge only this fixture command's stderr into the same live
                # byte-bounded stream. Retain diagnostics on failure privately.
                run.run(["sh", "-c", 'exec "$@" 2>&1', "infra-command", "docker", *args], output=output, **kw)
            except Exception:
                diagnostic = work / ("command-failure-" + secrets.token_hex(4) + ".log")
                diagnostic.write_bytes(output.getvalue())
                diagnostic.chmod(0o600)
                print("nginx command diagnostic:", diagnostic, flush=True)
                raise
            return output.getvalue()

        def owned_container(identifier):
            metadata = json.loads(docker("inspect", identifier))[0]
            self.assertEqual(metadata["Config"]["Labels"].get("knowoff.infra-test"), token)
            self.assertFalse(metadata["HostConfig"].get("PortBindings"))
            return metadata

        def request(container, path, *, host="api.knowoff.local", referer=""):
            # curl is supplied by the actual nginx base image. Test credentials
            # are unique synthetic sentinels; no real OAuth flow is invoked.
            return docker("exec", container, "curl", "-k", "--http1.1", "--max-time", "5", "-sS",
                          "-o", "/dev/null", "-w", "%{http_code}", "-H", "Host: " + host,
                          "-H", "Referer: " + referer, "https://127.0.0.1" + path).decode()

        try:
            docker("build", "--label", label, "-t", image, str(ROOT / "nginx"), timeout=600)
            image_created = True
            image_meta = json.loads(docker("image", "inspect", image))[0]
            self.assertEqual(image_meta["Config"]["Labels"].get("knowoff.infra-test"), token)
            network = docker("network", "create", "--internal", "--label", label, name).decode().strip()
            net = json.loads(docker("network", "inspect", network))[0]
            self.assertTrue(net["Internal"])
            self.assertEqual(net["Labels"].get("knowoff.infra-test"), token)
            stub_config = work / "upstream.conf"
            stub_config.write_text('events {}\nhttp { access_log off; server { listen 8080; listen 9090; location / { return 204; } } }\n')
            stub_config.chmod(0o600)
            stub = docker("create", "--name", name + "-upstream", "--label", label,
                          "--network", network, "--network-alias", "server", "--network-alias", "adminer",
                          "--entrypoint", "sh", image, "-c",
                          "nginx -c /fixture.conf && exec sleep 600").decode().strip()
            containers.append(stub)
            owned_container(stub)
            # Stream fixture bytes: the Docker daemon need not share the
            # workstation's /tmp namespace and receives no host-directory mount.
            archive = io.BytesIO()
            with tarfile.open(fileobj=archive, mode="w") as tar:
                item = tarfile.TarInfo("fixture.conf")
                item.mode = 0o600
                item.size = stub_config.stat().st_size
                tar.addfile(item, io.BytesIO(stub_config.read_bytes()))
            docker("cp", "-", stub + ":/", data=archive.getvalue())
            docker("start", stub)
            proxy = docker("create", "--name", name + "-proxy", "--label", label,
                           "--network", network, "--entrypoint", "sh", image, "-c",
                           "rm /var/log/nginx/access.log /var/log/nginx/error.log; exec /entrypoint.sh").decode().strip()
            containers.append(proxy)
            owned_container(proxy)
            docker("start", proxy)
            deadline = time.monotonic() + 15
            while True:
                # Startup readiness checks have an explicit bounded condition;
                # failed product requests below are never blindly retried.
                ready = docker("exec", proxy, "sh", "-c",
                               "if test -f /etc/nginx/certs/active/fullchain.pem && nc -z -w1 127.0.0.1 443; then echo ready; else echo waiting; fi")
                if ready.strip() == b"ready":
                    break
                self.assertLess(time.monotonic(), deadline, "nginx fixture did not become ready")
                time.sleep(0.1)
            docker("exec", proxy, "nginx", "-t")
            sentinels = ["code-" + token, "state-" + token, "referer-" + token]
            query = "/api/auth/oauth/callback?code=" + sentinels[0] + "&state=" + sentinels[1]
            referer = "https://provider.invalid/?secret=" + sentinels[2]
            self.assertEqual(request(proxy, query, referer=referer), "204")
            self.assertEqual(request(proxy, "/healthz?secret=" + sentinels[0], referer=referer), "204")
            retired_status = request(proxy, "/", host="minio.knowoff.local")
            self.assertEqual(retired_status, "204")  # Default API vhost; no MinIO backend exists.
            owned_container(stub)
            # Stop only the owned upstream process, retaining its network
            # interface so this is a refused connection, not an ARP timeout.
            docker("exec", stub, "nginx", "-c", "/fixture.conf", "-s", "stop")
            deadline = time.monotonic() + 5
            while docker("exec", stub, "sh", "-c",
                         "if nc -z -w1 127.0.0.1 8080; then echo listening; else echo stopped; fi").strip() != b"stopped":
                self.assertLess(time.monotonic(), deadline, "fixture upstream did not stop")
                time.sleep(0.1)
            self.assertEqual(request(proxy, query, referer=referer), "502")
            # The ordinary error path has no sensitive query and must still log.
            self.assertEqual(request(proxy, "/healthz"), "502")
            logs = docker("exec", proxy, "sh", "-c",
                          "cat /var/log/nginx/access.log /var/log/nginx/error.log")
            (work / "proxy.log").write_bytes(logs)
            (work / "proxy.log").chmod(0o600)
            for sentinel in sentinels:
                self.assertNotIn(sentinel.encode(), logs)
            self.assertNotIn(b"/api/auth/oauth/callback", logs)
            self.assertIn(b"GET /healthz HTTP/1.1", logs)
            self.assertIn(b"502", logs)
            self.assertIn(b"connect() failed", logs)
            self.assertNotIn(b"minio:9001", logs)
            self.assertEqual(len(json.loads(docker("network", "inspect", network))[0]["Containers"]), 2)
            evidence = {"syntax": "passed", "callback_success": 204, "callback_upstream_error": 502,
                        "sentinels_absent": True, "ordinary_diagnostics_present": True,
                        "image_id": image_meta["Id"], "minio_containers": 0, "published_ports": 0,
                        "http_version": "1.1",
                        "source_sha256": {str(p.relative_to(ROOT)): hashlib.sha256(p.read_bytes()).hexdigest()
                                          for p in [ROOT / "nginx/Dockerfile", ROOT / "nginx/nginx.conf",
                                                    ROOT / "nginx/entrypoint.sh", *sorted((ROOT / "nginx/conf.d").glob("*.conf"))]}}
            (work / "results.json").write_text(json.dumps(evidence, indent=2) + "\n")
            (work / "results.json").chmod(0o600)
            print("nginx fixture evidence:", work / "results.json", flush=True)
        finally:
            # Cleanup gets its own bounded budget and proves ownership before
            # every mutation. Never address an ordinary Compose resource.
            run = ops.CommandRunner(timeout=15, overall=120)
            failures = []
            # Discover by the unguessable ownership label too, covering an
            # uncertain create response that did not return its resource ID.
            try:
                containers.extend(docker("ps", "-aq", "--no-trunc", "--filter", "label=" + label).decode().split())
            except Exception as error:
                failures.append(type(error).__name__)
            for container in reversed(list(dict.fromkeys(containers))):
                try:
                    owned_container(container)
                    docker("rm", "-f", container)
                except Exception as error:
                    failures.append(type(error).__name__)
            if network:
                try:
                    net = json.loads(docker("network", "inspect", network))[0]
                    self.assertEqual(net["Labels"].get("knowoff.infra-test"), token)
                    self.assertFalse(net["Containers"])
                    docker("network", "rm", network)
                except Exception as error:
                    failures.append(type(error).__name__)
            if image_created:
                try:
                    meta = json.loads(docker("image", "inspect", image))[0]
                    self.assertEqual(meta["Config"]["Labels"].get("knowoff.infra-test"), token)
                    docker("image", "rm", image)
                except Exception as error:
                    failures.append(type(error).__name__)
            self.assertFalse(failures, "owned nginx fixture cleanup failures: " + ", ".join(failures))


if __name__ == "__main__":
    unittest.main()
