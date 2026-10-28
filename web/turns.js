// Turn buttons. Converse only: Listen has no Turns, so the group follows the
// Mode control. The server's `turn` event (including the auto-switch to them
// after my spoken output) drives the highlight through set().
// The same Mode control hides listen-only Language Pairs in Converse: a pair
// with one Direction cannot run there.

export function createTurns(container, onTurn = () => {}) {
  const buttons = new Map();
  for (const [who, label] of [['me', 'Me'], ['them', 'Them']]) {
    const button = document.createElement('button');
    button.type = 'button';
    button.textContent = label;
    button.addEventListener('click', () => onTurn(who));
    container.append(button);
    buttons.set(who, button);
  }

  const mode = document.getElementById('mode');
  const pairs = document.getElementById('pair');

  function applyMode() {
    const converse = mode?.value === 'converse';
    container.hidden = !converse;
    if (!pairs) return;
    let firstVisible = null;
    for (const option of pairs.options) {
      // shortcut: main.js marks a one-Direction pair with " (listen)"; replace
      // with a data-converse attribute when main.js is next edited.
      option.hidden = converse && option.textContent.endsWith(' (listen)');
      if (!option.hidden && firstVisible === null) firstVisible = option;
    }
    if (pairs.selectedOptions[0]?.hidden && firstVisible) pairs.value = firstVisible.value;
  }

  mode?.addEventListener('change', applyMode);
  // hello (and so the Pair options) can arrive after this module runs.
  if (pairs && typeof MutationObserver === 'function') {
    new MutationObserver(applyMode).observe(pairs, { childList: true });
  }
  applyMode();

  return {
    set(who) {
      for (const [key, button] of buttons) button.classList.toggle('active', key === who);
    },
    setMode(modeName) {
      if (mode) mode.value = modeName;
      applyMode();
    },
  };
}
