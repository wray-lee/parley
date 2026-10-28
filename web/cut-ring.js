// Cut indicator: the live Partial (greyed italic), a ring that fills over the
// 2.5 s accumulation window, and a cut mark left by the last Segment.
const ACCUMULATION_MS = 2500;
const NS = 'http://www.w3.org/2000/svg';
const RADIUS = 6;
const CIRCUMFERENCE = 2 * Math.PI * RADIUS;
// shortcut: 24 code points as "longer Segment"; revisit with real tuning.
const LONG_SEGMENT = 24;

export function createCutRing(element) {
  if (!element) return { set() {} };
  element.style.display = 'inline-flex';
  element.style.alignItems = 'center';
  element.style.gap = '0.35em';

  const partial = document.createElement('span');
  partial.className = 'partial';
  partial.style.color = 'var(--muted, #888)';
  partial.style.fontStyle = 'italic';
  partial.hidden = true;

  const ring = document.createElementNS(NS, 'svg');
  ring.setAttribute('viewBox', '0 0 16 16');
  ring.setAttribute('width', '14');
  ring.setAttribute('height', '14');
  ring.setAttribute('aria-hidden', 'true');
  ring.hidden = true;
  const arc = document.createElementNS(NS, 'circle');
  for (const [name, value] of Object.entries({
    cx: '8',
    cy: '8',
    r: String(RADIUS),
    fill: 'none',
    stroke: 'currentColor',
    'stroke-width': '2',
    'stroke-dasharray': String(CIRCUMFERENCE),
    'stroke-dashoffset': String(CIRCUMFERENCE),
    transform: 'rotate(-90 8 8)',
  })) {
    arc.setAttribute(name, value);
  }
  ring.append(arc);

  const mark = document.createElement('span');
  mark.className = 'cut-mark';
  mark.setAttribute('aria-hidden', 'true');
  mark.textContent = '✂';
  mark.hidden = true;

  element.append(partial, ring, mark);
  let animation = null;

  return {
    set(event) {
      if (event.type === 'asr_partial') {
        partial.textContent = event.text ?? '';
        partial.hidden = !event.text;
        if (!event.text) {
          ring.hidden = true;
          animation?.cancel();
          animation = null;
        } else if (ring.hidden) {
          ring.hidden = false;
          arc.setAttribute('stroke-dashoffset', String(CIRCUMFERENCE));
          animation = arc.animate(
            [{ strokeDashoffset: CIRCUMFERENCE }, { strokeDashoffset: 0 }],
            { duration: ACCUMULATION_MS, fill: 'forwards' },
          );
        }
        return;
      }
      if (event.type === 'segment') {
        animation?.cancel();
        animation = null;
        partial.hidden = true;
        ring.hidden = true;
        const length = [...(event.src ?? '')].length;
        mark.hidden = false;
        mark.style.marginLeft = length >= LONG_SEGMENT ? '0.75em' : '0.25em';
        element.title = `Cut after ${length} chars, ${event.latency_ms} ms`;
      }
    },
  };
}
