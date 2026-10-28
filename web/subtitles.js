// Bilingual subtitle list: source above translation, latency alongside.

export function createSubtitles(list, banner = null) {
  return {
    setDemo(visible) {
      if (banner) banner.hidden = !visible;
    },
    add(segment) {
      const item = document.createElement('li');

      const source = document.createElement('div');
      source.className = 'source';
      source.textContent = segment.src;

      const translation = document.createElement('div');
      translation.className = 'translation';
      translation.textContent = segment.tgt;

      const latency = document.createElement('span');
      latency.className = 'latency';
      latency.textContent = `${segment.latency_ms} ms`;

      item.append(source, translation, latency);
      list.append(item);
      list.scrollTop = list.scrollHeight;
      return item;
    },
  };
}
