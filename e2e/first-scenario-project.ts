// Prints the Playwright project name for a scenario's FIRST spec file (its
// entry state), so `make scenario-serve` can target only that project.
// Playwright always runs a project's `dependencies` in full regardless of
// --grep (grep only filters the requested project itself), so grepping
// "00 " across a chained scenario's whole run still executes every
// dependency project's later tests — --project=<first> avoids that by never
// resolving any dependency in the first place.
import { SCENARIOS } from './scenario-registry';

const name = process.argv[2] ?? '';
const scenario = SCENARIOS[name];
if (!scenario) {
  console.error(`Unknown scenario: ${name}`);
  process.exit(1);
}
process.stdout.write(scenario.specs[0].replace('.spec.ts', ''));
