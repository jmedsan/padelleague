export const STAGE_ORDER = ['created', 'dated', 'assigned', 'mid', 'end'] as const;
export type StageName = (typeof STAGE_ORDER)[number];

export interface Scenario {
  description: string;
  // 'blank' boots an empty server: the scenario's first spec seeds everything itself.
  startStage: StageName | 'blank';
  // Points the server's SMTP at Mailpit (also forced by MAIL=1 on any scenario).
  mail?: boolean;
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
  'mail-chat': {
    description: 'one match, four players with distinct emails, SMTP pointed at Mailpit (run `make mail`) — post a chat message and watch the inbox',
    startStage: 'blank',
    mail: true,
    specs: ['mail-chat.spec.ts'],
  },
  'mail-chat-scheduled': {
    description: 'as mail-chat, with a confirmed date, time and venue on the match — post a chat message and compare the email summary',
    startStage: 'blank',
    mail: true,
    specs: ['mail-chat-scheduled.spec.ts'],
  },
  'mail-chat-proposed': {
    description: 'as mail-chat, with a date, time and venue proposed by p01 and not accepted — post a chat message and compare the email summary',
    startStage: 'blank',
    mail: true,
    specs: ['mail-chat-proposed.spec.ts'],
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
