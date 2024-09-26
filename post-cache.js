
const DEFAULT_MAX_ITEMS = 1000000;
const DEFAULT_MAX_AGE_MS = 24 * 60 * 60 * 1000;

class PostCache {
  #map;
  #maxItems;
  #maxAge;

  constructor(maxItems=DEFAULT_MAX_ITEMS, maxAge=DEFAULT_MAX_AGE_MS) {
    this.#map = new Map();
    this.#maxItems = maxItems;
    this.#maxAge = maxAge;
  }

  set(now, k, v) {
    this.#map.set(k, { v, t: now });
    this.#cleanup(now);
  }

  update(now, k, v) {
    const found = this.#map.get(k);
    if (found !== undefined) {
      found.v = v;
    }
    this.#cleanup(now);
  }

  take(now, k) {
    let rv;
    let rec = this.#map.get(k);
    if (rec !== undefined) {
      this.#map.delete(k);
      if (rec.t >= now - this.#maxAge) {
        rv = {
          value: rec.v,
          age: now - rec.t,
        };
      }
    }
    this.#cleanup(now);
    return rv;
  }

  size() {
    return this.#map.size;
  }

  #cleanup(now) {
    let sizeOver = this.#map.size - this.#maxItems;
    const minTime = now - this.#maxAge;
    const toDelete = [];
    for (const [k, { t }] of this.#map) {
      if (sizeOver <= 0 && t >= minTime) break;
      sizeOver -= 1;
      toDelete.push(k);
    }
    toDelete.forEach(k => this.#map.delete(k));
  }
}

export default PostCache;
