import Database from 'better-sqlite3';

const DEFAULT_MAX_ITEMS = 1000000;
const DEFAULT_MAX_AGE_MS = 24 * 60 * 60 * 1000;
const DEFAULT_SQLITE_DB = ':memory:';



class PostCache {
  #maxItems;
  #maxAge;

  #db;
  #dbSet;
  #dbUpdate;
  #dbTake;
  #dbTrim;
  #dbCount;
  #dbOldest;
  #dbNewest;

  constructor({
    maxItems = DEFAULT_MAX_ITEMS,
    maxAge = DEFAULT_MAX_AGE_MS,
    sqliteDb = DEFAULT_SQLITE_DB,
  } = {}) {
    this.#maxItems = maxItems;
    this.#maxAge = maxAge;
    this.#db = new Database(sqliteDb, { timeout: 1000 });
    // thanks https://kerkour.com/sqlite-for-servers
    this.#db.pragma('journal_mode = WAL');
    this.#db.pragma('synchronous = NORMAL');
    this.#db.pragma(`cache_size = ${100 * Math.pow(2, 20)}`); // 100MiB
    this.#db.exec(`
      create table if not exists posts (
        rkey text    primary key,
        t    integer not null, -- not (necessarily) a unix timestamp!
        data text    not null -- json blob
      ) strict;
      create index if not exists posts_t on posts (t);
    `);

    this.#dbSet = this.#db.prepare(
      'insert into posts values (?, ?, ?) on conflict do nothing');

    this.#dbUpdate = this.#db.prepare(
      'update posts set data = ? where rkey = ?');

    this.#dbTake = this.#db.prepare(
      'delete from posts where rkey = ? returning t, data');

    this.#dbTrim = this.#db.prepare(`
      with cutoff as (
        select t
          from posts
         order by t desc
         limit 1
        offset ?
      )
      delete from posts
      where t < ? or t <= (select t from cutoff)
    `);

    this.#dbCount = this.#db.prepare(
      'select count(rowid) as count from posts');

    this.#dbOldest = this.#db.prepare(
      'select t from posts order by t limit 1');

    this.#dbNewest = this.#db.prepare(
      'select t from posts order by t desc limit 1');
  }

  set(now, k, v) {
    this.#dbSet.run(k, now, JSON.stringify(v));
  }

  update(now, k, v) {
    this.#dbUpdate.run(JSON.stringify(v), k);
  }

  take(now, k) {
    const rec = this.#dbTake.get(k);
    if (!rec || rec.t < now - this.#maxAge) return;
    let value;
    try {
      value = JSON.parse(rec.data);
    } catch (e) {
      console.error('failed to parse json data from cache db', rec);
      return;
    }
    return {
      value: JSON.parse(rec.data),
      age: now - rec.t,
    };
  }

  size() {
    return this.#dbCount.get()?.count;
  }

  oldest() {
    return this.#dbOldest.get()?.t;
  }

  newest() {
    return this.#dbNewest.get()?.t;
  }

  trim(now) {
    this.#dbTrim.run(this.#maxItems, now - this.#maxAge);
  }
}

export default PostCache;
