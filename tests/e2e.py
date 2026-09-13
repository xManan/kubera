#!/usr/bin/env python3
"""End-to-end test driver for the Kubera MCP server.

Spawns the compiled binary over stdio and exercises every tool: create,
duplicate detection, updates, voiding, categories, reports, pagination, and
error mapping. Run: go build -o kubera ./cmd/kubera && python3 tests/e2e.py
"""

import json
import os
import select
import subprocess
import sys
import tempfile

BIN = sys.argv[1] if len(sys.argv) > 1 else "./kubera"
PASS = []
FAIL = []


def check(name, cond, detail=""):
    if cond:
        PASS.append(name)
        print(f"  ok   {name}")
    else:
        FAIL.append(name)
        print(f"  FAIL {name}  {detail}")


class Server:
    def __init__(self):
        self.dir = tempfile.mkdtemp(prefix="kubera-e2e-")
        env = dict(os.environ, KUBERA_DATABASE_PATH=os.path.join(self.dir, "kubera.db"),
                   KUBERA_LOG_LEVEL="error")
        self.proc = subprocess.Popen(
            [BIN], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
            stderr=subprocess.PIPE, env=env, text=True, bufsize=1)
        self.buf = {}
        self.next_id = 0

    def send(self, method, params):
        self.next_id += 1
        msg = {"jsonrpc": "2.0", "id": self.next_id, "method": method, "params": params}
        self.proc.stdin.write(json.dumps(msg) + "\n")
        self.proc.stdin.flush()
        return self.next_id

    def notify(self, method, params):
        self.proc.stdin.write(json.dumps({"jsonrpc": "2.0", "method": method, "params": params}) + "\n")
        self.proc.stdin.flush()

    def recv(self, want_id, timeout=15):
        if want_id in self.buf:
            return self.buf.pop(want_id)
        while True:
            ready, _, _ = select.select([self.proc.stdout], [], [], timeout)
            if not ready:
                raise TimeoutError(f"no response for id {want_id}")
            line = self.proc.stdout.readline()
            if not line:
                raise RuntimeError(f"server exited: {self.proc.stderr.read()}")
            msg = json.loads(line)
            mid = msg.get("id")
            if mid == want_id:
                return msg
            if mid is not None:
                self.buf[mid] = msg

    def call(self, tool, args):
        rid = self.send("tools/call", {"name": tool, "arguments": args})
        res = self.recv(rid)["result"]
        sc = res.get("structuredContent")
        return (sc or {}), bool(res.get("isError"))

    def stop(self):
        self.proc.stdin.close()
        self.proc.wait(timeout=10)
        return self.proc.returncode


def main():
    s = Server()

    # --- handshake ---
    rid = s.send("initialize", {"protocolVersion": "2025-06-18", "capabilities": {},
                                "clientInfo": {"name": "e2e", "version": "0"}})
    info = s.recv(rid)["result"]["serverInfo"]
    s.notify("notifications/initialized", {})
    check("initialize returns kubera server", info["name"] == "kubera", info)

    rid = s.send("tools/list", {})
    tools = {t["name"] for t in s.recv(rid)["result"]["tools"]}
    want = {"create_transaction", "get_transaction", "list_transactions", "update_transaction",
            "void_transaction", "list_categories", "create_category", "update_category",
            "archive_category", "get_daily_summary", "get_monthly_summary",
            "get_transaction_summary", "get_category_breakdown"}
    check("tools/list exposes all 13 tools", tools == want, tools ^ want)

    # --- categories ---
    out, err = s.call("create_category", {"name": "Eating Out"})
    check("create_category", not err and out["status"] == "category_created", out)
    eat_id = out["category"]["id"]

    out, err = s.call("create_category", {"name": "eating  out"})
    check("duplicate category rejected case/whitespace-insensitively",
          err and out["error"]["code"] == "category_already_exists", out)

    out, err = s.call("create_category", {"name": "Salary"})
    sal_id = out["category"]["id"]
    check("create second category", not err, out)

    out, err = s.call("create_category", {"name": "x" * 101})
    check("category name over 100 chars rejected", err and out["error"]["code"] == "invalid_request", out)

    # --- create transactions ---
    orig_text = "Spent Rs.198 On HDFC Bank Card xx1234"
    out, err = s.call("create_transaction", {
        "direction": "money_out", "amount_minor": 19800, "currency": "INR",
        "occurred_at": "2026-09-13T14:04:54Z", "category_id": eat_id,
        "description": "EATCLUB BRANDS PRIVATE", "original_notification": orig_text})
    check("create_transaction", not err and out["status"] == "created", out)
    t1 = out["transaction"]
    check("transaction id is stable txn_ prefix", t1["id"].startswith("txn_"), t1["id"])
    check("original notification preserved verbatim", t1["original_text"] == orig_text, t1)
    check("timestamps normalized to UTC Z", t1["occurred_at"] == "2026-09-13T14:04:54Z", t1["occurred_at"])

    # retry: same amount/direction/currency, desc differs only by case+spacing, within 5 min
    out, err = s.call("create_transaction", {
        "direction": "money_out", "amount_minor": 19800, "currency": "INR",
        "occurred_at": "2026-09-13T14:07:30Z", "category_id": eat_id,
        "description": "  eatclub   brands  private "})
    check("likely duplicate detected, not written",
          not err and out["status"] == "duplicate", out)
    check("duplicate names existing transaction", out.get("existing_transaction", {}).get("id") == t1["id"], out)
    check("duplicate match reason is field match",
          out.get("match_reason") == "amount_currency_direction_description_time", out)

    out, err = s.call("create_transaction", {
        "direction": "money_out", "amount_minor": 19800, "currency": "INR",
        "occurred_at": "2026-09-13T18:04:54Z", "category_id": eat_id,
        "description": "EATCLUB BRANDS PRIVATE"})
    check("same amount 4h later is NOT a duplicate", not err and out["status"] == "created", out)
    t2 = out["transaction"]

    # reference-id match beats differing fields
    out, err = s.call("create_transaction", {
        "direction": "money_in", "amount_minor": 5000000, "currency": "INR",
        "occurred_at": "2026-09-13T09:00:00Z", "category_id": sal_id,
        "description": "Salary credit September", "reference_id": "REF-99"})
    t3 = out["transaction"]
    check("create with reference id", not err and out["status"] == "created", out)
    # reference-id match (LLD: same reference + currency + direction) beats
    # differing amount and description; opposite direction does NOT match.
    out, err = s.call("create_transaction", {
        "direction": "money_out", "amount_minor": 42, "currency": "INR",
        "occurred_at": "2026-09-13T20:00:00Z", "category_id": eat_id,
        "description": "Totally different thing", "reference_id": "REF-99"})
    check("opposite direction with same reference is a new transaction",
          not err and out["status"] == "created", out)
    t_wrong_dir = out["transaction"]
    out, err = s.call("create_transaction", {
        "direction": "money_in", "amount_minor": 42, "currency": "INR",
        "occurred_at": "2026-09-13T20:00:00Z", "category_id": eat_id,
        "description": "Totally different thing", "reference_id": "REF-99"})
    check("reference id match wins over differing fields",
          not err and out["status"] == "duplicate" and out["match_reason"] == "reference_id", out)
    check("reference duplicate points at REF-99 transaction",
          out.get("existing_transaction", {}).get("id") == t3["id"], out)
    out, err = s.call("void_transaction", {"transaction_id": t_wrong_dir["id"], "reason": "e2e cleanup"})
    check("cleanup void", not err, out)

    # multi-currency
    out, err = s.call("create_transaction", {
        "direction": "money_out", "amount_minor": 499, "currency": "USD",
        "occurred_at": "2026-09-13T15:00:00Z", "category_id": eat_id,
        "description": "Subscription"})
    check("create USD transaction", not err and out["status"] == "created", out)
    t_usd = out["transaction"]

    # --- validation errors (no DB writes) ---
    for tool, args, code in [
        ("create_transaction", {"direction": "sideways", "amount_minor": 100, "currency": "INR",
                                "occurred_at": "2026-09-13T10:00:00Z", "category_id": eat_id},
         "invalid_direction"),
        ("create_transaction", {"direction": "money_out", "amount_minor": 0, "currency": "INR",
                                "occurred_at": "2026-09-13T10:00:00Z", "category_id": eat_id},
         "invalid_amount"),
        ("create_transaction", {"direction": "money_out", "amount_minor": 100, "currency": "inr",
                                "occurred_at": "2026-09-13T10:00:00Z", "category_id": eat_id},
         "invalid_currency"),
        ("create_transaction", {"direction": "money_out", "amount_minor": 100, "currency": "INR",
                                "occurred_at": "2026-09-13T10:00:00", "category_id": eat_id},
         "invalid_timestamp"),
        ("create_transaction", {"direction": "money_out", "amount_minor": 100, "currency": "INR",
                                "occurred_at": "2026-09-13T10:00:00Z", "category_id": "cat_nope"},
         "category_not_found"),
        ("get_transaction", {"transaction_id": "txn_missing"}, "transaction_not_found"),
    ]:
        out, err = s.call(tool, args)
        check(f"{code} mapped safely", err and out["error"]["code"] == code
              and "sql" not in json.dumps(out).lower(), out)

    # --- update ---
    out, err = s.call("update_transaction", {
        "transaction_id": t2["id"], "amount_minor": 19950, "description": "EATCLUB"})
    check("update_transaction", not err and out["transaction"]["amount_minor"] == 19950, out)
    check("update preserves id and original text",
          out["transaction"]["id"] == t2["id"] and out["transaction"]["original_text"] == "", out["transaction"])
    out, err = s.call("update_transaction", {"transaction_id": t2["id"]})
    check("empty patch rejected with empty_update",
          err and out["error"]["code"] == "empty_update", out)

    # --- void ---
    out, err = s.call("void_transaction", {"transaction_id": t2["id"], "reason": "double counted"})
    check("void_transaction", not err and out["status"] == "voided", out)
    check("voided transaction carries voided_at and reason",
          out["transaction"]["voided_at"] and out["transaction"]["void_reason"] == "double counted", out)
    out, err = s.call("void_transaction", {"transaction_id": t2["id"]})
    check("repeated void is typed error, not duplicate audit",
          err and out["error"]["code"] == "transaction_already_voided", out)
    out, err = s.call("update_transaction", {"transaction_id": t2["id"], "amount_minor": 1})
    check("updates rejected after voiding",
          err and out["error"]["code"] == "transaction_already_voided", out)

    # --- archived category rules ---
    out, err = s.call("archive_category", {"category_id": sal_id})
    check("archive_category", not err and out["status"] == "category_archived", out)
    out, err = s.call("archive_category", {"category_id": sal_id})
    check("archive is idempotent", not err and out["status"] == "category_archived", out)
    out, err = s.call("create_transaction", {
        "direction": "money_in", "amount_minor": 100, "currency": "INR",
        "occurred_at": "2026-09-13T10:00:00Z", "category_id": sal_id})
    check("archived category cannot back new transactions",
          err and out["error"]["code"] == "archived_category", out)
    out, err = s.call("update_transaction", {"transaction_id": t1["id"], "category_id": sal_id})
    check("archived category cannot be assigned by update",
          err and out["error"]["code"] == "archived_category", out)
    out, err = s.call("list_categories", {})
    check("default listing hides archived",
          [c["id"] for c in out["categories"]] == [eat_id], out)
    out, err = s.call("list_categories", {"include_archived": True})
    check("include_archived shows archived, name order kept",
          [c["id"] for c in out["categories"]] == [eat_id, sal_id], out)

    # --- list + pagination ---
    out, err = s.call("list_transactions", {"limit": 2})
    check("list defaults to non-voided", not err and len(out["transactions"]) == 2, out)
    page1 = [t["id"] for t in out["transactions"]]
    out, err = s.call("list_transactions", {"limit": 2, "cursor": out["next_cursor"]})
    page2 = [t["id"] for t in out["transactions"]]
    check("cursor pagination has no overlap, newest first",
          not (set(page1) & set(page2)) and out["has_more"] is False, (page1, page2))
    out, err = s.call("list_transactions", {"include_voided": True, "direction": "money_out"})
    ids = [t["id"] for t in out["transactions"]]
    check("voided included on demand and direction filter works",
          t2["id"] in ids and t3["id"] not in ids, ids)
    out, err = s.call("list_transactions", {"start": "2026-09-14T00:00:00Z", "end": "2026-09-13T00:00:00Z"})
    check("inverted range rejected", err and out["error"]["code"] == "invalid_date_range", out)

    # --- reports ---
    out, err = s.call("get_daily_summary", {"date": "2026-09-13"})
    check("daily summary excludes voided", not err, out)
    totals = {t["currency"]: t for t in out["currency_totals"]}
    check("currencies never summed together", set(totals) == {"INR", "USD"}, totals)
    inr = totals["INR"]
    # INR non-voided money_out: 19800 (t1) + 19950 (t2 after update, voided -> excluded)
    # money_out active: t1 19800 + USD n/a; money_in: t3 5000000
    check("daily INR totals: in=5000000 out=19800 net=4980200 count=2",
          inr["money_in_minor"] == 5000000 and inr["money_out_minor"] == 19800
          and inr["net_minor"] == 4980200 and inr["transaction_count"] == 2, inr)
    check("daily USD total isolated", totals["USD"]["money_out_minor"] == 499, totals)
    cat_names = {(c["category_id"], c["direction"], c["currency"]): c["total_minor"] for c in out["categories"]}
    check("category breakdown separates direction and currency",
          cat_names[(eat_id, "money_out", "INR")] == 19800
          and cat_names[(eat_id, "money_out", "USD")] == 499
          and cat_names[(sal_id, "money_in", "INR")] == 5000000, cat_names)

    out, err = s.call("get_monthly_summary", {"year": 2026, "month": 9})
    check("monthly summary includes largest transactions",
          not err and len(out.get("largest_transactions", [])) >= 3
          and out["largest_transactions"][0]["amount_minor"] >= out["largest_transactions"][1]["amount_minor"], out)
    out, err = s.call("get_monthly_summary", {"year": 2026, "month": 13})
    check("month 13 rejected", err and out["error"]["code"] == "invalid_date_range", out)

    out, err = s.call("get_transaction_summary", {
        "start": "2026-09-01T00:00:00Z", "end": "2026-10-01T00:00:00Z"})
    check("custom range summary", not err and totals["INR"]["money_in_minor"]
          == out["currency_totals"][0]["money_in_minor"], out)
    out, err = s.call("get_category_breakdown", {
        "start": "2026-09-13T00:00:00Z", "end": "2026-09-14T00:00:00Z"})
    check("category breakdown tool", not err and len(out["categories"]) == 3, out)

    # --- graceful shutdown on stdin EOF ---
    rc = s.stop()
    check("clean shutdown on stdin EOF", rc == 0, rc)

    print(f"\n{len(PASS)} passed, {len(FAIL)} failed")
    if FAIL:
        print("failed:", ", ".join(FAIL))
        sys.exit(1)


if __name__ == "__main__":
    main()
