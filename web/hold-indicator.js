// Hold indicator: the translator's ⏸ answer and a spinner while a Segment is
// Held, waiting for the next Cut or the 2.5 s Forced Translation.

export function createHoldIndicator(element) {
  if (!element) return { set() {} };

  element.setAttribute('role', 'status');
  element.setAttribute('aria-live', 'polite');
  element.style.display = 'inline-flex';
  element.style.alignItems = 'center';
  element.style.gap = '0.35em';
  element.style.color = 'var(--muted, #888)';

  const answer = document.createElement('span');
  answer.textContent = '⏸';
  answer.setAttribute('aria-hidden', 'true');
  const spinner = document.createElement('span');
  spinner.textContent = '◌';
  spinner.setAttribute('aria-hidden', 'true');
  const label = document.createElement('span');
  label.textContent = 'held';
  element.append(answer, spinner, label);
  element.hidden = true;

  let animation = null;

  function stop() {
    element.hidden = true;
    animation?.cancel();
    animation = null;
  }

  return {
    set(event) {
      if (event?.type === 'hold') {
        if (!event.held) {
          stop();
          return;
        }
        element.hidden = false;
        label.textContent = event.merges
          ? `held · ${event.merges} merge${event.merges === 1 ? '' : 's'}`
          : 'held';
        if (!animation) {
          animation = spinner.animate?.(
            [{ transform: 'rotate(0deg)' }, { transform: 'rotate(360deg)' }],
            { duration: 1200, iterations: Infinity },
          ) ?? null;
        }
        return;
      }
      if (event?.type === 'segment' || event?.type === 'state') stop();
    },
  };
}
