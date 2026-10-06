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
  'leveled-16-start': {
    description: 'published, ready to play — 24 pending, 0 finals (continuation of pregen)',
    startStage: 'assigned',
    specs: ['leveled-16-start.spec.ts'],
  },
  'leveled-16': {
    description: 'published baseline: play, top-up, release, pair filter',
    startStage: 'assigned',
    specs: ['leveled-16.spec.ts'],
  },
  'leveled-16-full': {
    description: 'nodates → pregen → start → baseline, incremental on one server',
    startStage: 'created',
    specs: ['leveled-16-nodates.spec.ts', 'leveled-16-pregen.spec.ts', 'leveled-16-start.spec.ts', 'leveled-16.spec.ts'],
  },
  'result-ready': {
    description: 'match with date and place, ready for the result input',
    startStage: 'assigned',
    specs: ['result-ready.spec.ts'],
  },
  'result-counter': {
    description: 'rival proposed a result — reject and counter-propose from the rival\'s side',
    startStage: 'assigned',
    specs: ['result-counter.spec.ts'],
  },
  'result-provisional': {
    description: 'a result was proposed and the rival has not answered — counted everywhere with the unconfirmed warning',
    startStage: 'assigned',
    specs: ['result-provisional.spec.ts'],
  },
  'result-disputes': {
    description: 'three matches of one pair: a lone proposal (counts, warns), a disputed one and a deadlock of conflicting proposals (count nowhere)',
    startStage: 'assigned',
    specs: ['result-disputes.spec.ts'],
  },
  'result-carried': {
    description: 'match with 6-3 carried over, date and place set — ready for the rest of the result',
    startStage: 'assigned',
    specs: ['result-carried.spec.ts'],
  },
  'result-nodate': {
    description: 'match with no date and place proposed — the result cannot be entered yet',
    startStage: 'assigned',
    specs: ['result-nodate.spec.ts'],
  },
  'result-date-proposed': {
    description: 'a date and place were proposed, not accepted — the result cannot be entered yet',
    startStage: 'assigned',
    specs: ['result-date-proposed.spec.ts'],
  },
  'several-competitions': {
    description: 'three active competitions with quorum 12h, 48h, 48h',
    startStage: 'created',
    specs: ['several-competitions.spec.ts'],
  },
  'smtp-verify': {
    description: 'SMTP sink receives finalization email',
    startStage: 'assigned',
    specs: ['smtp-verify.spec.ts'],
  },
};
