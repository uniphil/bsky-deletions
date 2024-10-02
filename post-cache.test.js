import PostCache from './post-cache.js';

test('the cache holds things', () => {
  const cache = new PostCache();
  expect(cache.take(0, 'x')).toBeUndefined();
  cache.set(0, 'x', 1);
  expect(cache.take(0, 'x')).toStrictEqual({ value: 1, age: 0 });
  expect(cache.take(0, 'x')).toBeUndefined();
});

test('the cache knows the age of things', () => {
  const cache = new PostCache();
  cache.set(0, 'x', 'a');
  cache.set(1, 'y', 'b');
  expect(cache.take(1, 'x')).toStrictEqual({ value: 'a', age: 1 });
  expect(cache.take(1, 'y')).toStrictEqual({ value: 'b', age: 0 });
});

test('the cache limits how many items it holds', () => {
  const cache = new PostCache({ maxItems: 1 });
  cache.set(0, 'x', 'a');
  cache.set(0, 'y', 'b');
  expect(cache.take(0, 'x')).toBeUndefined();
  expect(cache.take(0, 'y')).toStrictEqual({ value: 'b', age: 0 });
});

test('the cache removes too-old items', () => {
  const cache = new PostCache({ maxItems: 10000, maxAge: 1 });
  cache.set(0, 'x', 'a');
  cache.set(1, 'y', 'b');
  cache.set(2, 'z', 'c');
  expect(cache.take(2, 'x')).toBeUndefined();
  expect(cache.take(2, 'y')).toStrictEqual({ value: 'b', age: 1 });
  expect(cache.take(2, 'z')).toStrictEqual({ value: 'c', age: 0 });
});

test('the cache updates existing items', () => {
  const cache = new PostCache();
  cache.set(0, 'x', 'a');
  cache.update(1, 'x', 'b');
  expect(cache.take(2, 'x')).toStrictEqual({ value: 'b', age: 2 });
  expect(cache.take(2, 'x')).toBeUndefined();
});

test('the cache reports its size', () => {
  const cache = new PostCache();
  expect(cache.size()).toBe(0);
  cache.set(0, 'x', 'a');
  expect(cache.size()).toBe(1);
  cache.take(0, 'x');
  expect(cache.size()).toBe(0);
});

test('the cache reports oldest member', () => {
  const cache = new PostCache({ maxItems: 2 });
  expect(cache.oldest()).toBeUndefined();
  cache.set(0, 'x', 'a');
  cache.set(1, 'y', 'b');
  expect(cache.oldest()).toBe(0);
  cache.set(2, 'z', 'c');
  expect(cache.oldest()).toBe(1);
});
