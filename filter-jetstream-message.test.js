import filter, { redact } from './filter-jetstream-message';

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
            "uri": "at://did:plc:xxxxxx/app.bsky.feed.post/3l4yZZZZZZZZZ"
          },
          "root": {
            "cid": "asdfasdfasdfasdf",
            "uri": "at://did:plc:xxxxxx/app.bsky.feed.post/3l4yZZZZZZZZZ"
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

test('the redactor should redact link text', () => {
  expect(redact(
    "【BTS】Rebuilding the fantastical realm for an unforgettable experience!!! | Hero is Back | YOUKU\n\nhttps://www.example.com/0123456789\n\n【BTS】Rebuilding the fantastical realm for an unforgettable experience!!! | Hero is Back | YOUKU",
    [
      {
        "features": [
          {
            "$type": "app.bsky.richtext.facet#link",
            "uri": "https://www.example.com/0123456789"
          }
        ],
        "index": {
          "byteEnd": 135,
          "byteStart": 101
        }
      }
    ]
  )).toBe(
    '【BTS】Rebuilding the fantastical realm for an unforgettable experience!!! | Hero is Back | YOUKU\n\nwww.████████████\n\n【BTS】Rebuilding the fantastical realm for an unforgettable experience!!! | Hero is Back | YOUKU'
  );
});

test('the redactor should redact mentions', () => {
  expect(redact(
    "Parabéns @unnaxoficial.bsky.social pela vaga conquistada pra final 👏🏻👏🏻👏🏻👏🏻🦋❤️ \n#EstrelaDaCasa",
    [
      {
        "$type": "app.bsky.richtext.facet",
        "features": [
          {
            "$type": "app.bsky.richtext.facet#mention",
            "did": "did:plc:xxxxxx"
          }
        ],
        "index": {
          "byteEnd": 35,
          "byteStart": 10
        }
      },
      {
        "features": [
          {
            "$type": "app.bsky.richtext.facet#tag",
            "tag": "EstrelaDaCasa"
          }
        ],
        "index": {
          "byteEnd": 126,
          "byteStart": 112
        }
      }
    ]
  )).toBe(
    'Parabéns @████████████ pela vaga conquistada pra final 👏🏻👏🏻👏🏻👏🏻🦋❤️ \n#EstrelaDaCasa'
  );
});

test('the redactor should redact multiple mentions', () => {
  expect(redact(
    "test @one.bsky.social two @lookitup.baby three.",
    [
      {
        "$type": "app.bsky.richtext.facet",
        "features": [
          {
            "$type": "app.bsky.richtext.facet#mention",
            "did": "did:plc:xxxxxx"
          }
        ],
        "index": {
          "byteEnd": 40,
          "byteStart": 26
        }
      },
      {
        "$type": "app.bsky.richtext.facet",
        "features": [
          {
            "$type": "app.bsky.richtext.facet#mention",
            "did": "did:plc:xxxxxx"
          }
        ],
        "index": {
          "byteEnd": 21,
          "byteStart": 5
        }
      }
    ]
  )).toBe(
    'test @████████████ two @████████████ three.'
  );
});
