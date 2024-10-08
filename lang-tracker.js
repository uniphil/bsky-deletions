
const MIN_POULARITY_RATIO = 1000;

class LangTracker {
  #map
  constructor() {
    this.#map = new Map();
  }
  addSighting(lang) {
    this.#map.set(lang, (this.#map.get(lang) ?? 0) + 1);
  }
  getActive() {
    const mostPopularLangSightings = Math.max(...this.#map.values());
    const sightingsThreshold = Math.floor(mostPopularLangSightings / 300);
    return Array.from(this.#map.entries())
      .filter(([_, c]) => c > sightingsThreshold)
      .sort(([_ka, a], [_kb, b]) => b - a)
      .map(([k, _]) => k);
  }
  _getStats() {
    return Object.fromEntries(this.#map);
  }
}

export default LangTracker;
