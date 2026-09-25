export const STAGE_ORDER = ['created', 'dated', 'assigned', 'mid', 'end'] as const;
export type StageName = (typeof STAGE_ORDER)[number];

export interface Scenario {
  description: string;
  startStage: StageName;
  specs: string[];
}

export const SCENARIOS: Record<string, Scenario> = {
  'leveled-16-nodates': {
    description: '16 pairs, no dates — generate refused until dates are set',
    startStage: 'created',
    specs: ['leveled-16-nodates.spec.ts'],
  },
  'leveled-16-pregen': {
    description: 'dates already set, not generated — admin clicks Generar + publishes through the UI',
    startStage: 'dated',
    specs: ['leveled-16-pregen.spec.ts'],
  },
  'leveled-16': {
    description: 'published baseline: play, top-up, release, pair filter',
    startStage: 'assigned',
    specs: ['leveled-16.spec.ts'],
  },
  'leveled-16-full': {
    description: 'nodates → pregen → baseline, incremental on one server',
    startStage: 'created',
    specs: ['leveled-16-nodates.spec.ts', 'leveled-16-pregen.spec.ts', 'leveled-16.spec.ts'],
  },
  'smtp-verify': {
    description: 'SMTP sink receives finalization email',
    startStage: 'assigned',
    specs: ['smtp-verify.spec.ts'],
  },
};
