(function () {
  'use strict';
  const { S } = window.WB;
  // the create button is called "Erstellen" everywhere (the frame keeps its place and shape)
  Object.keys(S).forEach((k) => { const f = S[k]; S[k] = () => { const r = f(); Object.keys(r).forEach((d) => { r[d] = r[d].replace(/(<span class="mk-capture"[^>]*>(?:<svg[^]*?<\/svg>))Erfassen/g, '$1Erstellen').replace(/(<div class="[^"]*cap[^"]*">(?:<svg[^]*?<\/svg>))Erfassen<\/div>/g, '$1Erstellen</div>'); }); return r; }; });
})();
