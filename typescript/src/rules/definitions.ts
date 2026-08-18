import type { Definition } from "./types.js";

export const ruleIDs = {
  readmeRequired: "repo.readme-required",
  agentsRequired: "repo.agents-required",
  agentsEmpty: "repo.agents-empty",
  agentsCommandsRequired: "repo.agents-commands-required",
  agentsCommandInvalid: "repo.agents-command-invalid",
  agentsScopeRequired: "repo.agents-scope-required",
  agentsStale: "repo.agents-stale",
  ciRequired: "repo.ci-required",
  slophammerCiRequired: "repo.slophammer-ci-required",
  tsPackageRequired: "ts.package-required",
  tsTypecheckRequired: "ts.typecheck-required",
  tsStrictRequired: "ts.strict-required",
  tsNoExplicitAny: "ts.no-explicit-any",
  tsNoUnsafeTypes: "ts.no-unsafe-types",
  tsLintRequired: "ts.lint-required",
  tsFormatRequired: "ts.format-required",
  tsTestRequired: "ts.test-required",
  tsCoverageRequired: "ts.coverage-required",
  tsComplexityRequired: "ts.complexity-required",
  tsDryRequired: "ts.dry-required",
  tsMutationRequired: "ts.mutation-required",
  tsDependencyBoundariesRequired: "ts.dependency-boundaries-required",
  tsScopeIncomplete: "ts.scope-incomplete",
  tsSuppressionsJustified: "ts.suppressions-justified"
} as const;

export const defaultDefinitions: readonly Definition[] = [
  {
    id: ruleIDs.readmeRequired,
    title: "README required",
    category: "repo",
    severity: "error",
    path: "README.md",
    message: "README.md is required",
    description: "The target repo should have a README.md.",
    status: "implemented"
  },
  {
    id: ruleIDs.agentsRequired,
    title: "Agent instructions required",
    category: "repo",
    severity: "error",
    path: "AGENTS.md",
    message: "AGENTS.md is required",
    description: "The target repo should have an AGENTS.md.",
    status: "implemented"
  },
  {
    id: ruleIDs.agentsEmpty,
    title: "Useful agent instructions required",
    category: "repo",
    severity: "error",
    path: "AGENTS.md",
    message: "AGENTS.md must contain useful repository instructions",
    description:
      "The root AGENTS.md must contain useful text after Markdown headings, comments, and whitespace are removed.",
    status: "implemented"
  },
  {
    id: ruleIDs.agentsCommandsRequired,
    title: "Agent verification commands required",
    category: "repo",
    severity: "error",
    path: "AGENTS.md",
    message: "AGENTS.md must name a verification command supported by the repository",
    description:
      "When verification commands can be derived from repository evidence, the governing AGENTS.md must name at least one of them.",
    status: "implemented"
  },
  {
    id: ruleIDs.agentsCommandInvalid,
    title: "Generated agent commands must be valid",
    category: "repo",
    severity: "error",
    path: "AGENTS.md",
    message: "AGENTS.md contains generated commands without repository evidence",
    description:
      "Commands in a Slophammer-managed AGENTS.md evidence block must still be supported by current repository evidence.",
    status: "implemented"
  },
  {
    id: ruleIDs.agentsScopeRequired,
    title: "Package agent instructions required",
    category: "repo",
    severity: "error",
    path: "AGENTS.md",
    message: "Package instructions must name a verification command for this package",
    description:
      "Each package boundary must be governed by an AGENTS.md that names a root umbrella command or a command derived for that package.",
    status: "implemented"
  },
  {
    id: ruleIDs.agentsStale,
    title: "Generated agent evidence must be current",
    category: "repo",
    severity: "error",
    path: "AGENTS.md",
    message: "The Slophammer AGENTS.md evidence block is stale",
    description:
      "A Slophammer-managed AGENTS.md evidence block must match the package and command facts derived from the current repository.",
    status: "implemented"
  },
  {
    id: ruleIDs.ciRequired,
    title: "CI workflow required",
    category: "repo",
    severity: "error",
    path: ".github/workflows",
    message: ".github/workflows must contain at least one .yml or .yaml workflow",
    description: "The target repo should have a CI workflow under .github/workflows.",
    status: "implemented"
  },
  {
    id: ruleIDs.slophammerCiRequired,
    title: "Slophammer enforcement required",
    category: "repo",
    severity: "error",
    path: ".github/workflows",
    message: "CI must run a Slophammer checker when slophammer.yml is present",
    description:
      "A repository that carries slophammer.yml must execute a Slophammer checker from binding CI evidence; config without enforcement is decoration.",
    status: "implemented"
  },
  {
    id: ruleIDs.tsPackageRequired,
    title: "TypeScript package required",
    category: "typescript",
    severity: "error",
    path: "package.json",
    message: "TypeScript projects must include package.json",
    description: "TypeScript projects should include package.json.",
    status: "implemented"
  },
  {
    id: ruleIDs.tsTypecheckRequired,
    title: "TypeScript typecheck required",
    category: "typescript",
    severity: "error",
    path: ".github/workflows",
    message: "TypeScript projects must declare a no-emit typecheck in CI or scripts",
    description:
      "TypeScript projects should declare tsc, tsgo, or an equivalent no-emit typecheck.",
    tool: "tsc/tsgo --noEmit",
    status: "implemented"
  },
  {
    id: ruleIDs.tsStrictRequired,
    title: "TypeScript strict mode required",
    category: "typescript",
    severity: "error",
    path: "tsconfig.json",
    message: "TypeScript projects must enable strict mode",
    description: "TypeScript projects should enable strict compiler settings.",
    tool: "tsc",
    status: "implemented"
  },
  {
    id: ruleIDs.tsNoExplicitAny,
    title: "No explicit any",
    category: "typescript",
    severity: "error",
    path: "eslint.config.mjs",
    message: "TypeScript projects must reject explicit any",
    description: "TypeScript projects should configure lint rules to reject explicit any.",
    tool: "eslint/oxlint",
    status: "implemented"
  },
  {
    id: ruleIDs.tsNoUnsafeTypes,
    title: "No unsafe type operations",
    category: "typescript",
    severity: "error",
    path: "eslint.config.mjs",
    message: "TypeScript projects must reject unsafe type operations",
    description:
      "TypeScript projects should configure lint rules to reject unsafe assignments, calls, member access, and returns.",
    tool: "eslint/oxlint",
    status: "implemented"
  },
  {
    id: ruleIDs.tsLintRequired,
    title: "TypeScript lint required",
    category: "typescript",
    severity: "error",
    path: ".github/workflows",
    message: "TypeScript projects must declare a linter in CI or scripts",
    description:
      "TypeScript projects should declare ESLint, Oxlint, Biome, or an equivalent linter.",
    tool: "eslint/oxlint/biome",
    status: "implemented"
  },
  {
    id: ruleIDs.tsFormatRequired,
    title: "TypeScript formatter required",
    category: "typescript",
    severity: "error",
    path: ".github/workflows",
    message: "TypeScript projects must declare a formatter check",
    description:
      "TypeScript projects should declare a formatter check through Prettier, Oxfmt, Biome, Dprint, or an equivalent tool.",
    tool: "prettier/oxfmt/biome/dprint",
    status: "implemented"
  },
  {
    id: ruleIDs.tsTestRequired,
    title: "TypeScript tests required",
    category: "typescript",
    severity: "error",
    path: ".github/workflows",
    message: "TypeScript projects must declare tests in CI or scripts",
    description: "TypeScript projects should declare a real test runner command.",
    tool: "node --test/vitest/jest",
    status: "implemented"
  },
  {
    id: ruleIDs.tsCoverageRequired,
    title: "TypeScript coverage gate required",
    category: "typescript",
    severity: "error",
    path: ".github/workflows",
    message: "TypeScript projects must declare a coverage gate",
    description: "TypeScript projects should declare a coverage gate with a target of at least 85.",
    tool: "c8/nyc/vitest/jest coverage",
    status: "implemented"
  },
  {
    id: ruleIDs.tsComplexityRequired,
    title: "TypeScript complexity required",
    category: "typescript",
    severity: "error",
    path: "eslint.config.mjs",
    message: "TypeScript projects must enforce complexity limits",
    description:
      "TypeScript projects should enforce complexity limits through a configured linter.",
    tool: "eslint/oxlint/biome",
    status: "implemented"
  },
  {
    id: ruleIDs.tsDryRequired,
    title: "TypeScript DRY check required",
    category: "typescript",
    severity: "error",
    path: ".github/workflows",
    message: "TypeScript projects must declare a DRY check",
    description:
      "TypeScript projects should declare Slophammer's native copied-block duplicate detector.",
    tool: "slophammer-ts dry",
    status: "implemented"
  },
  {
    id: ruleIDs.tsMutationRequired,
    title: "TypeScript mutation check required",
    category: "typescript",
    severity: "error",
    path: ".github/workflows",
    message: "TypeScript projects must declare mutation testing",
    description:
      "TypeScript projects should declare TypeScript mutation testing, normally through StrykerJS. Only executing invocations count: list, scan, check, dry-run, and manifest-only forms cannot fail on a surviving mutant and are not evidence. Stryker exits zero on surviving mutants unless thresholds.break is set, so runs count only beside a positive breaking threshold.",
    tool: "stryker",
    status: "implemented"
  },
  {
    id: ruleIDs.tsDependencyBoundariesRequired,
    title: "TypeScript dependency boundaries required",
    category: "typescript",
    severity: "error",
    path: "slophammer.yml",
    message: "TypeScript projects must respect configured dependency boundaries",
    description:
      "TypeScript projects should declare dependency boundaries in slophammer.yml and keep imports inside them.",
    status: "implemented"
  },
  {
    id: ruleIDs.tsScopeIncomplete,
    title: "TypeScript scope completeness",
    category: "typescript",
    severity: "error",
    path: "slophammer.yml",
    message:
      "Configured TypeScript scope must cover all production files or exclude them with reasons",
    description:
      "Every production TypeScript file must be in configured scope or covered by a conventional or reasoned exclude, so findings cannot be hidden by narrowing scope.",
    status: "implemented"
  },
  {
    id: ruleIDs.tsSuppressionsJustified,
    title: "TypeScript suppressions justified",
    category: "typescript",
    severity: "error",
    path: ".",
    message: "lint and type suppressions in production TypeScript code must carry a description",
    description:
      "eslint-disable, @ts-ignore, @ts-expect-error, biome-ignore, and oxlint-disable directives in production scope must carry a description; bare suppressions accumulate silently.",
    status: "implemented"
  }
];
