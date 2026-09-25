import { SCENARIOS } from './scenario-registry';

console.log('\nAvailable scenarios:\n');
for (const [name, sc] of Object.entries(SCENARIOS)) {
  console.log(`  ${name.padEnd(22)} ${sc.description}`);
  console.log(`  ${''.padEnd(22)} stage: ${sc.startStage}, specs: ${sc.specs.join(' → ')}`);
}
console.log('\nUsage: make e2e-scenario SCENARIO=<name>\n');
process.exit(1);
