import unittest
from plan import build
class PlanTest(unittest.TestCase):
    def row(self,**kw):
        r=dict(path='/source/Book.m4b',relative_path='Book.m4b',size=36000000,duration=3600,audio=[{'channels':1,'codec_name':'aac'}],issues=[],chapters=[{}],embedded_cover=True);r.update(kw);return r
    def test_already_small_audio_is_retained(self):
        self.assertEqual(build([self.row(size=14000000)])['jobs'][0]['action'],'keep_low_bitrate_source')
    def test_missing_art_blocks_conversion(self):
        self.assertEqual(build([self.row(issues=['missing_cover'],embedded_cover=False)])['jobs'][0]['action'],'resolve_cover_before_conversion')
    def test_unreadable_audio_requires_review(self):
        self.assertEqual(build([self.row(error='Permission denied')])['jobs'][0]['action'],'manual_review')
    def test_opus_is_not_transcoded_again(self):
        self.assertEqual(build([self.row(audio=[{'channels':1,'codec_name':'opus'}])])['jobs'][0]['action'],'keep_existing_opus')
if __name__=='__main__':unittest.main()
