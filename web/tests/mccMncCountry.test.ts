import test from 'node:test'
import assert from 'node:assert/strict'
import { lookupHomeCountryISO, type HomeOperatorLike, type MccMncRow } from '../src/utils/mcc-mnc'

const rows: MccMncRow[] = [
  { mcc: '454', mnc: '00', iso: 'hk', country: 'Hong Kong', country_code: '852', network: 'CSL' },
  { mcc: '454', mnc: '03', iso: 'hk', country: 'Hong Kong', country_code: '852', network: '3' },
  { mcc: '460', mnc: '01', iso: 'cn', country: 'China', country_code: '86', network: 'China Unicom' },
  { mcc: '262', mnc: '01', iso: 'de', country: 'Germany', country_code: '49', network: 'Telekom' },
  { mcc: '901', mnc: '01', iso: 'gb', country: '', country_code: '', network: '' },
  { mcc: '901', mnc: '02', iso: 'us', country: '', country_code: '', network: '' }
]
const index = new Map(rows.map(row => [row.mcc + row.mnc, row]))

const cases: { name: string; modem: HomeOperatorLike | undefined; expected: string }[] = [
  { name: 'complete native PLMN has precedence', modem: { native_mcc: '454', native_mnc: '00', imsi: '262011234567890' }, expected: 'hk' },
  { name: 'native MCC works without MNC', modem: { native_mcc: '454', imsi: '262011234567890' }, expected: 'hk' },
  { name: 'unknown MNC still identifies the country', modem: { native_mcc: '454', native_mnc: '99' }, expected: 'hk' },
  { name: 'reported IMSI works without native fields', modem: { imsi: '454003063191853' }, expected: 'hk' },
  { name: 'country does not require guessing MNC length', modem: { imsi: '454033123456789' }, expected: 'hk' },
  { name: 'whitespace is normalized', modem: { imsi: ' 454003063191853 ' }, expected: 'hk' },
  { name: 'empty identity', modem: {}, expected: '' },
  { name: 'no modem', modem: undefined, expected: '' },
  { name: 'all-nine IMSI is rejected', modem: { imsi: '999999999999999' }, expected: '' },
  { name: 'malformed IMSI is rejected', modem: { imsi: '45400invalid' }, expected: '' },
  { name: 'short IMSI is rejected', modem: { imsi: '4540' }, expected: '' },
  { name: 'overlong IMSI is rejected', modem: { imsi: '4540012345678901' }, expected: '' },
  { name: 'unknown MCC', modem: { imsi: '999001234567890' }, expected: '' },
  { name: 'ambiguous international MCC', modem: { imsi: '901991234567890' }, expected: '' }
]

for (const { name, modem, expected } of cases) {
  test(name, () => assert.equal(lookupHomeCountryISO(index, modem), expected))
}

test('roaming network is not used as the SIM country', () => {
  const roaming = { imsi: '454003063191853', operator: 'China Unicom', mcc: '460', mnc: '01' }
  assert.equal(lookupHomeCountryISO(index, roaming), 'hk')
  assert.equal(lookupHomeCountryISO(index, { ...roaming, imsi: '' }), '')
})

test('switching identity does not retain the previous country', () => {
  assert.equal(lookupHomeCountryISO(index, { imsi: '454003063191853' }), 'hk')
  assert.equal(lookupHomeCountryISO(index, { imsi: '' }), '')
  assert.equal(lookupHomeCountryISO(index, { imsi: '262011234567890' }), 'de')
})

test('an unavailable table does not fabricate a country', () => {
  assert.equal(lookupHomeCountryISO(null, { imsi: '454003063191853' }), '')
  assert.equal(lookupHomeCountryISO(new Map(), { imsi: '454003063191853' }), '')
})
