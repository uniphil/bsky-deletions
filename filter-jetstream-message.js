const redact = (text, facets) => {
  if (!facets) return text;

  // this whole function is sketchy but the following line is the sketchiest:
  let sourceBytes = Buffer.from(text, 'utf8');
  // sketchy because we've round-tripped utf8 -> js string (via json parse) -> utf8
  // hopefully the bytes made it through that unscathed!
  // ... but also the worst case we're probably looking at here is some redaction offset errors

  let lastEnd = 0;
  let redactedText = '';

  // https://docs.bsky.app/docs/advanced-guides/post-richtext
  const facetsToRedact = facets
    // 0. discard facets we don't care about
    .filter(facet =>
      facet.features.some(f =>
        ['app.bsky.richtext.facet#mention', 'app.bsky.richtext.facet#link'].includes(f.$type)))
    // 1. sort facets by start index
    .sort(({index: {byteStart: a}}, {index: {byteStart: b}}) => a - b)
    .forEach(({ features, index: { byteStart, byteEnd } }) => {
      // 2. discard any facets that overlap eachother
      if (byteStart < lastEnd) return;
      // 3. apply redactions
      redactedText += sourceBytes.slice(lastEnd, byteStart).toString();
      redactedText += features.some(f => f.$type === 'app.bsky.richtext.facet#mention') ? '@' : 'www.';
      redactedText += '█████████';
      lastEnd = byteEnd;
    });

  redactedText += sourceBytes.slice(lastEnd).toString();
  return redactedText;
};

const filter = contents => {
  if (contents.type !== 'com') return;
  const t = Math.floor(contents.time_us / 1000);
  const { type: commitType, rkey, record } = contents.commit ?? {};
  if (['c', 'u'].includes(commitType)) {
    const { text, embed, facets, langs, reply } = record;
    let type;
    if (commitType === 'c') {
      if (text === '') return;
      type = 'post';
    } else if (commitType === 'u') {
      if (text === '') return { type: 'delete', t, rkey };
      type = 'update';
    }
    const redacted = redact(text, facets);
    const normLangs = langs?.map(lang => lang.split('-')[0].toLowerCase());
    let target = null;
    if (reply !== undefined) {
      target = 'reply';
    } else if (embed && embed.$type === 'app.bsky.embed.record') {
      target = 'quote';
    }
    return { type, t, rkey, langs: normLangs, text: redacted, target };
  } else if (commitType === 'd') {
    return { type: 'delete', t, rkey };
  }
}

export { redact };
export default filter;
