import LangTracker from './lang-tracker.js';

test('should track languages seen', () => {
  const tracker = new LangTracker();
  expect(tracker.getActive()).toStrictEqual([]);
  tracker.addSighting('en');
  expect(tracker.getActive()).toStrictEqual(['en']);
  tracker.addSighting('pt');
  expect(tracker.getActive()).toStrictEqual(['en', 'pt']);
});

test('should order languages by most active', () => {
  const tracker = new LangTracker();
  tracker.addSighting('en');
  tracker.addSighting('pt');
  tracker.addSighting('pt');
  expect(tracker.getActive()).toStrictEqual(['pt', 'en']);
});

test('should exclude very infrequent languages', () => {
  const tracker = new LangTracker();
  for (var i = 0; i < 2000; i++) {
    tracker.addSighting('pt');
  }
  tracker.addSighting('hu');
  expect(tracker.getActive()).toStrictEqual(['pt']);
});
