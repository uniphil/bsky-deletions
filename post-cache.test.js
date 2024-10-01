import PostCache from './post-cache.js';

test('the cache holds things', async () => {
  const cache = new PostCache();
  expect(await cache.take(0, 'x')).toBeUndefined();
  await cache.set(0, 'x', 1);
  expect(await cache.take(0, 'x')).toStrictEqual({ value: 1, age: 0 });
  expect(await cache.take(0, 'x')).toBeUndefined();
});

test('the cache knows the age of things', async () => {
  const cache = new PostCache();
  await cache.set(0, 'x', 'a');
  await cache.set(1, 'y', 'b');
  expect(await cache.take(1, 'x')).toStrictEqual({ value: 'a', age: 1 });
  expect(await cache.take(1, 'y')).toStrictEqual({ value: 'b', age: 0 });
});

test('the cache limits how many items it holds', async () => {
  const cache = new PostCache(1);
  await cache.set(0, 'x', 'a');
  await cache.set(0, 'y', 'b');
  expect(await cache.take(0, 'x')).toBeUndefined();
  expect(await cache.take(0, 'y')).toStrictEqual({ value: 'b', age: 0 });
});

test('the cache removes too-old items', async () => {
  const cache = new PostCache(10000, 1);
  await cache.set(0, 'x', 'a');
  await cache.set(1, 'y', 'b');
  await cache.set(2, 'z', 'c');
  expect(await cache.take(2, 'x')).toBeUndefined();
  expect(await cache.take(2, 'y')).toStrictEqual({ value: 'b', age: 1 });
  expect(await cache.take(2, 'z')).toStrictEqual({ value: 'c', age: 0 });
});

test('the cache updates existing items', async () => {
  const cache = new PostCache();
  await cache.set(0, 'x', 'a');
  cache.update(1, 'x', 'b');
  expect(await cache.take(2, 'x')).toStrictEqual({ value: 'b', age: 2 });
  expect(await cache.take(2, 'x')).toBeUndefined();
});

test('the cache reports its size', async () => {
  const cache = new PostCache();
  expect(await cache.size()).toBe(0);
  await cache.set(0, 'x', 'a');
  expect(await cache.size()).toBe(1);
  await cache.take(0, 'x');
  expect(await cache.size()).toBe(0);
});

test('the cache reports oldest member', async () => {
  const cache = new PostCache(2);
  expect(await cache.oldest()).toBeUndefined();
  await cache.set(0, 'x', 'a');
  await cache.set(1, 'y', 'b');
  expect(await cache.oldest()).toBe(0);
  await cache.set(2, 'z', 'c');
  expect(await cache.oldest()).toBe(1);
});
