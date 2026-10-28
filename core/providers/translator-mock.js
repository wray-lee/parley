// Returns the sample's reference translation, always prefixed with the DEMO marker.

const MARKER = '[DEMO] ';

export function create({ references = {}, marker = MARKER } = {}) {
  return {
    name: 'mock',
    async *translate({ text }) {
      yield marker + (references[text] ?? text);
    },
  };
}
