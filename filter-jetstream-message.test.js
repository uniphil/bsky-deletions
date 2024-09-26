import filter from './filter-jetstream-message';

test('the filter should map creation to posts', () => {
  expect(filter({
    "did": "did:plc:xxxxxxxxx",
    "time_us": 1711111116933838,
    "type": "com",
    "commit": {
      "rev": "3l4yXXXXXXXXX",
      "type": "c",
      "collection": "app.bsky.feed.post",
      "rkey": "3l4yYYYYYYYY",
      "record": {
        "$type": "app.bsky.feed.post",
        "createdAt": "2024-01-01T00:00:00.000Z",
        "langs": [
          "en"
        ],
        "text": "In a world of naps be a medically-induced coma ✨"
      },
      "cid": "asdfasdfasdfasdfasdfasdf"
    }
  })).toStrictEqual({
    type: 'post',
    t: 1711111116933,
    rkey: '3l4yYYYYYYYY',
    langs: ['en'],
    text: 'In a world of naps be a medically-induced coma ✨',
    target: null,
  });
});

test('the filter should normalize langs', () => {
  expect(filter({
    "did": "did:plc:xxxxxxxxx",
    "time_us": 1711111116933838,
    "type": "com",
    "commit": {
      "rev": "3l4yXXXXXXXXX",
      "type": "c",
      "collection": "app.bsky.feed.post",
      "rkey": "3l4yYYYYYYYY",
      "record": {
        "$type": "app.bsky.feed.post",
        "createdAt": "2024-01-01T00:00:00.000Z",
        "langs": [
          "en-US"
        ],
        "text": "In a world of naps be a medically-induced coma ✨"
      },
      "cid": "asdfasdfasdfasdfasdfasdf"
    }
  })).toStrictEqual({
    type: 'post',
    t: 1711111116933,
    rkey: '3l4yYYYYYYYY',
    langs: ['en'],
    text: 'In a world of naps be a medically-induced coma ✨',
    target: null,
  });
});



test('the filter should identify replies', () => {
  expect(filter({
    "did": "did:plc:xxxxxxxxx",
    "time_us": 1711111116933838,
    "type": "com",
    "commit": {
      "rev": "3l4yXXXXXXXXX",
      "type": "c",
      "collection": "app.bsky.feed.post",
      "rkey": "3l4yYYYYYYYY",
      "record": {
        "$type": "app.bsky.feed.post",
        "createdAt": "2024-01-01T00:00:00.000Z",
        "langs": [
          "en"
        ],
        "reply": {
          "parent": {
            "cid": "asdfasdfasdfasdf",
            "uri": "at://did:plc:u6mavbfl2p54ycmipyeihdb3/app.bsky.feed.post/3l4yZZZZZZZZZ"
          },
          "root": {
            "cid": "asdfasdfasdfasdf",
            "uri": "at://did:plc:u6mavbfl2p54ycmipyeihdb3/app.bsky.feed.post/3l4yZZZZZZZZZ"
          }
        },
        "text": "more is coming."
      },
      "cid": "asdfasdfasdfasdfasdfasdfasdf"
    }
  })).toStrictEqual({
    type: 'post',
    t: 1711111116933,
    rkey: '3l4yYYYYYYYY',
    langs: ['en'],
    text: 'more is coming.',
    target: 'reply',
  });
});
