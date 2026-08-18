import { mkdtemp, readFile, symlink, writeFile } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

import { describe, expect, test } from "vitest";

import { agentsFindings, deriveEvidence, renderAgents } from "../src/agents/agents.js";
import { run } from "../src/cli/cli.js";
import { newSnapshot } from "../src/repo/repo.js";

function snapshot(files: Readonly<Record<string, string>>) {
  return newSnapshot(
    "/repo",
    Object.entries(files).map(([filePath, content]) => ({ path: filePath, content }))
  );
}

describe("AGENTS.md evidence", () => {
  test("derives runner and package commands", () => {
    const evidence = deriveEvidence(
      snapshot({
        Makefile: "check:\n\t@true\n",
        "go/go.mod": "module example.com/demo\n",
        "rust/Cargo.toml": "[workspace]\nmembers = []\n",
        "python/pyproject.toml": '[dependency-groups]\ndev = ["pytest"]\n',
        "python/uv.lock": "version = 1\n",
        "web/package.json": '{"scripts":{"check":"true"}}',
        "web/pnpm-lock.yaml": "lockfileVersion: 9\n",
        "fixtures/demo/go.mod": "module example.com/ignored\n"
      })
    );

    expect(evidence.globalCommands).toEqual(["make check"]);
    expect(evidence.allCommands).toEqual(
      expect.arrayContaining([
        "go test ./...",
        "cargo test --workspace",
        "uv run pytest",
        "pnpm run check"
      ])
    );
    expect(evidence.packages.map((area) => area.path)).not.toContain("fixtures/demo");
    expect(renderAgents(snapshot({}))).toContain("No package manifest was detected");
  });

  test("uses workspace package managers", () => {
    const lockEvidence = deriveEvidence(
      snapshot({
        "pnpm-lock.yaml": "lockfileVersion: 9\n",
        "packages/app/package.json": '{"scripts":{"check":"true"}}'
      })
    );
    expect(lockEvidence.allCommands).toContain("pnpm run check");

    const metadataEvidence = deriveEvidence(
      snapshot({
        "package.json": '{"packageManager":"yarn@4.1.0"}',
        "packages/app/package.json": '{"scripts":{"build":"true"}}'
      })
    );
    expect(metadataEvidence.allCommands).toContain("yarn run build");
  });

  test("reports empty, missing commands, scope, invalid commands, and stale evidence", () => {
    expect(agentsFindings(snapshot({})).map((finding) => finding.rule_id)).toEqual([
      "repo.agents-required"
    ]);
    expect(
      agentsFindings(snapshot({ "AGENTS.md": "# Agents\n" })).map((finding) => finding.rule_id)
    ).toEqual(["repo.agents-empty"]);
    expect(
      agentsFindings(
        snapshot({
          "AGENTS.md": "# Agents\n\nKeep changes small and reviewable.\n",
          "package.json": '{"scripts":{"check":"true"}}'
        })
      ).map((finding) => finding.rule_id)
    ).toEqual(["repo.agents-commands-required"]);
    expect(
      agentsFindings(
        snapshot({
          "AGENTS.md": "# Agents\n\nRun `npm test` before finishing.\n",
          "packages/app/package.json": '{"scripts":{"test":"true"}}',
          "packages/worker/package.json": '{"scripts":{"build":"true"}}'
        })
      )
    ).toContainEqual(
      expect.objectContaining({
        rule_id: "repo.agents-scope-required",
        path: "packages/worker/AGENTS.md"
      })
    );

    const stale = [
      "# AGENTS.md",
      "",
      "Keep changes small and reviewable.",
      "",
      "<!-- slophammer:agents:start -->",
      "## Repository checks",
      "",
      "```sh",
      "npm test",
      "```",
      "<!-- slophammer:agents:end -->",
      ""
    ].join("\n");
    expect(
      agentsFindings(snapshot({ "AGENTS.md": stale })).map((finding) => finding.rule_id)
    ).toEqual(["repo.agents-command-invalid", "repo.agents-stale"]);
  });

  test("treats a supported command as useful content", () => {
    expect(
      agentsFindings(
        snapshot({
          "AGENTS.md": "```sh\ngo test ./...\n```\n",
          "go.mod": "module example.com/demo\n"
        })
      )
    ).toEqual([]);
  });

  test("accepts generated evidence", () => {
    const initial = snapshot({ "package.json": '{"scripts":{"check":"true"}}' });
    const generated = renderAgents(initial);
    expect(
      agentsFindings(
        snapshot({
          "AGENTS.md": generated,
          "package.json": '{"scripts":{"check":"true"}}'
        })
      )
    ).toEqual([]);
  });
});

describe("agents CLI", () => {
  test("writes, refuses, forces, previews, and checks", async () => {
    const root = await mkdtemp(path.join(os.tmpdir(), "slophammer-agents-ts-"));
    await writeFile(path.join(root, "Makefile"), "check:\n\t@true\n");

    const preview = await run(["agents", "init", root, "--dry-run"]);
    expect(preview).toMatchObject({ code: 0, stderr: "" });
    expect(preview.stdout).toContain("make check");

    expect(await run(["agents", "init", root])).toMatchObject({ code: 0 });
    const refused = await run(["agents", "init", root]);
    expect(refused.code).toBe(2);
    expect(refused.stderr).toContain("--force");

    await writeFile(path.join(root, "AGENTS.md"), "old\n");
    expect(await run(["agents", "init", root, "--force"])).toMatchObject({ code: 0 });
    expect(await readFile(path.join(root, "AGENTS.md"), "utf8")).toContain("make check");

    const checked = await run(["agents", "check", root, "--format", "json"]);
    expect(checked.code).toBe(0);
    expect(checked.stdout).toContain('"ok": true');
  });

  test("rejects forced symlink replacement", async () => {
    const root = await mkdtemp(path.join(os.tmpdir(), "slophammer-agents-ts-link-"));
    const externalRoot = await mkdtemp(path.join(os.tmpdir(), "slophammer-agents-ts-outside-"));
    const external = path.join(externalRoot, "external.md");
    await writeFile(external, "keep\n");
    await symlink(external, path.join(root, "AGENTS.md"));

    const result = await run(["agents", "init", root, "--force"]);
    expect(result.code).toBe(2);
    expect(result.stderr).toContain("symlink");
    expect(await readFile(external, "utf8")).toBe("keep\n");
  });

  test("rejects invalid agents command arguments", async () => {
    await expect(run(["agents", "check", "--wat"])).resolves.toMatchObject({ code: 2 });
    await expect(run(["agents", "init", ".", ".."])).resolves.toMatchObject({ code: 2 });
    await expect(run(["agents", "wat"])).resolves.toMatchObject({ code: 2 });
  });
});
