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
      'insert into posts values (?, ?, ?)');

    this.#dbUpdate = this.#db.prepare(
      'update posts set data = ? where rkey = ?');

    this.#dbTake = this.#db.prepare(
      'delete from posts where rkey = ? returning t, data');

    this.#dbTrim = this.#db.prepare(`
      with cutoff as (
        select rowid
          from posts
         order by rowid desc -- should work for a pretty long time
         limit 1
        offset ?
      )
      delete from posts
      where t < ? or rowid <= (select rowid from cutoff)
    `);

    this.#dbCount = this.#db.prepare(
      'select count(rowid) as count from posts');

    this.#dbOldest = this.#db.prepare(
      'select t from posts order by rowid limit 1');
  }

  set(now, k, v) {
    this.#dbSet.run(k, now, JSON.stringify(v));
    this.#cleanup(now);
  }

  update(now, k, v) {
    this.#dbUpdate.run(JSON.stringify(v), k);
    this.#cleanup(now);
  }

  take(now, k) {
    let rv;
    let rec = this.#dbTake.get(k);
    if (rec !== undefined) {
      if (rec.t >= now - this.#maxAge) {
        let value;
        try {
          value = JSON.parse(rec.data);
        } catch (e) {
          console.error('failed to parse json data from cache db', rec);
          return;
        }
        rv = {
          value: JSON.parse(rec.data),
          age: now - rec.t,
        };
      }
    }
    this.#cleanup(now);
    return rv;
  }

  size() {
    return this.#dbCount.get()?.count;
  }

  oldest() {
    return this.#dbOldest.get()?.t;
  }

  #cleanup(now) {
    this.#dbTrim.run(this.#maxItems, now - this.#maxAge);
  }
}

export default PostCache;
