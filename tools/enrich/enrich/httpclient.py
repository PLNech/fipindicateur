"""Polite, rate-limited JSON HTTP client for the Wikidata/Wikipedia APIs.

One shared client enforces a minimum interval between real network calls and
retries transient failures with exponential backoff. Cache hits never touch
this client, so the rate limit only paces genuine outbound requests.
"""

from __future__ import annotations

import time
from typing import Any

import requests

from . import __version__

USER_AGENT = (
    f"fipindicateur-enrich/{__version__} "
    "(local companion tool for fipindicateur; https://github.com/fipindicateur; "
    "contact via project repository)"
)


class NetworkError(RuntimeError):
    """Raised when a request keeps failing after all retries."""


class HttpClient:
    def __init__(
        self,
        min_interval: float = 1.0,
        max_retries: int = 3,
        timeout: float = 20.0,
        verbose: bool = False,
    ) -> None:
        self.min_interval = min_interval
        self.max_retries = max_retries
        self.timeout = timeout
        self.verbose = verbose
        self._last = 0.0
        self.calls = 0
        self._session = requests.Session()
        self._session.headers.update({"User-Agent": USER_AGENT})

    def _throttle(self) -> None:
        wait = self.min_interval - (time.monotonic() - self._last)
        if wait > 0:
            time.sleep(wait)

    def get_json(self, url: str, params: dict[str, Any]) -> dict[str, Any]:
        """GET a JSON endpoint, throttled and retried. Raises NetworkError."""
        last_exc: Exception | None = None
        for attempt in range(self.max_retries):
            self._throttle()
            self.calls += 1
            try:
                resp = self._session.get(url, params=params, timeout=self.timeout)
                self._last = time.monotonic()
                if resp.status_code == 429 or resp.status_code >= 500:
                    raise NetworkError(f"HTTP {resp.status_code}")
                resp.raise_for_status()
                return resp.json()
            except (requests.RequestException, NetworkError, ValueError) as exc:
                self._last = time.monotonic()
                last_exc = exc
                backoff = 2.0 ** attempt
                if self.verbose:
                    print(f"  request failed (attempt {attempt + 1}): {exc}; "
                          f"backoff {backoff:.1f}s")
                if attempt < self.max_retries - 1:
                    time.sleep(backoff)
        raise NetworkError(f"giving up after {self.max_retries} attempts: {last_exc}")
