import unittest
from pathlib import Path
from gpmc import Client, utils

auth_data = "androidId=31eee46f207ea2b6&lang=zh-CN&google_play_services_version=254632032&sdk_version=32&device_country=cn&it_caveat_types=2&app=com.google.android.apps.photos&oauth2_foreground=1&Email=xinkerrrr%40gmail.com&pkgVersionCode=49224303&token_request_options=CAA4AVABYAA%3D&client_sig=24bb24c05e47e0aefa68a58a766179d9b613a600&Token=aas_et%2FAKppINa1M7hZ_lv2bFlFGSnLfkB3VLoezCsrt8-jiOQiruu-jydXqLXOqfFD_IEzeb3iAIAOVdmLspNvs4HwqdRxaZlsnJWQZre7v_SblpThf6IZTeHzdWuH9r5_GwEuFGHiY_tXdIIIZNNU9SOc2cR40iap8H9FbNZe4QLVdFqn_u6UcnlFn6YjINVuB1X4-6l9cbT3vy_sHIYqfl0yOwI%3D&consumerVersionCode=49224303&check_email=1&service=oauth2%3Aopenid%20https%3A%2F%2Fwww.googleapis.com%2Fauth%2Fmobileapps.native%20https%3A%2F%2Fwww.googleapis.com%2Fauth%2Fphotos.native&assertion_jwt=eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9.eyJuYW1lc3BhY2UiOiJUb2tlbkJpbmRpbmciLCJhdWQiOiJodHRwczpcL1wvYWNjb3VudHMuZ29vZ2xlLmNvbVwvYWNjb3VudG1hbmFnZXIiLCJpc3MiOiIzdG1GX2VZdFd2NGFnNHU3VTF5VVA0cVJ2MDFDQmhFU2h4VWJleEQycmR3IiwiaWF0IjoxNzY3MTYyNDU3LCJlcGhlbWVyYWxfa2V5Ijp7Imt0eSI6InR5cGUuZ29vZ2xlYXBpcy5jb21cL2dvb2dsZS5jcnlwdG8udGluay5FY2llc0FlYWRIa2RmUHVibGljS2V5IiwiVGlua0tleXNldFB1YmxpY0tleUluZm8iOiJDT2FEX1lNTUV0MEJDdEFCQ2oxMGVYQmxMbWR2YjJkc1pXRndhWE11WTI5dEwyZHZiMmRzWlM1amNubHdkRzh1ZEdsdWF5NUZZMmxsYzBGbFlXUklhMlJtVUhWaWJHbGpTMlY1RW93QkVrUUtCQWdDRUFNU09oSTRDakIwZVhCbExtZHZiMmRzWldGd2FYTXVZMjl0TDJkdmIyZHNaUzVqY25sd2RHOHVkR2x1YXk1QlpYTkhZMjFMWlhrU0FoQVFHQUVZQVJvaEFIcW9sclc2OThuSWEyYTRQYkhySm5JMmJPVjVMY2lnSHJVVk5kdlh2cEdoSWlFQUtQUG84WXRHQ2NXX1BldHNCMEpxSXFyS2hqbzM2ZzZuWTJjbzU0Y1JDZ0lZQXhBQkdPYURfWU1NSUFFIn19.K5EGAEAuceIEM42IlRgbDW_geNVrYwCa923bl4GyiazW91dBZ7jDsJ7FSis8kgQjhkfZ1phAehOAt3VzizZe4g&callerPkg=com.google.android.apps.photos&check_tb_upgrade_eligible=1&callerSig=24bb24c05e47e0aefa68a58a766179d9b613a600"

class TestUpload(unittest.TestCase):
    def setUp(self):

        self.image_file_path = "media/image.png"
        self.image_sha1_hash_b64 = "bjvmULLYvkVj8jWVQFu1Pl98hYA="
        self.image_sha1_hash_hxd = "6e3be650b2d8be4563f23595405bb53e5f7c8580"
        self.directory_path = "D:\\google_photos_mobile_client-proto-decode-error-handling\\media"
        self.mkv_file_path = "D:\\google_photos_mobile_client-proto-decode-error-handling\\media\\sample_640x360.mkv"
        self.client = Client(auth_data=auth_data)

    def test_restore_from_trash(self):
        """Test restore from trash."""
        dedup_key = utils.urlsafe_base64(self.image_sha1_hash_b64)
        output = self.client.api.restore_from_trash([dedup_key])
        print(output)

    def test_get_download_urls(self):
        """Test get library data."""
        output = self.client.api.get_download_urls("AF1QipOD9PerDX6wrOoWHZKt0361PlyACUJrm8H4NHI")
        print(output)

    def test_set_archived(self):
        """Test get library data."""
        dedup_key = utils.urlsafe_base64(self.image_sha1_hash_b64)
        self.client.api.set_archived([dedup_key], is_archived=False)

    def test_set_favorite(self):
        """Test get library data."""
        dedup_key = utils.urlsafe_base64(self.image_sha1_hash_b64)
        self.client.api.set_favorite(dedup_key, is_favorite=False)

    def test_get_thumbnail(self):
        """Test get library data."""
        self.client.api.get_thumbnail("AF1QipOD9PerDX6wrOoWHZKt0361PlyACUJrm8H4NHI", width=500)

    def test_cache_upate(self):
        """Test get library data."""
        self.client.update_cache()

    def test_set_caption(self):
        """Test filter."""
        dedup_key = utils.urlsafe_base64(self.image_sha1_hash_b64)
        self.client.api.set_item_caption(dedup_key=dedup_key, caption="foobar")

    def test_filter(self):
        """Test filter."""
        response = self.client.upload(target=self.directory_path, filter_exp="copy", filter_ignore_case=True, filter_regex=True)
        print(response)

    def test_add_to_album(self):
        """Test add to album."""
        response = self.client.add_to_album(
            media_keys=["AF1QipPLS4p_aG90bX2qQV6QxH103avlaz5NAfiSR_X1", "AF1QipMvXu56uuldoyflKD60lctos9u-8BJ_luropFcZ"],
            album_name="TEST",
            show_progress=True,
        )
        print(response)

    def test_move_to_trash(self):
        """Test move to trash."""
        response = self.client.move_to_trash(sha1_hashes=self.image_sha1_hash_hxd)
        print(response)

    def test_image_upload(self):
        """Test image upload."""
        media_key = self.client.upload(target=self.image_file_path, force_upload=True, show_progress=True, saver=True, use_quota=True)
        print(media_key)

    def test_directory_uplod(self):
        """Test directory upload."""
        media_key = self.client.upload(target=self.directory_path, threads=5, show_progress=True)
        print(media_key)

    def test_image_upload_with_hash(self):
        """Test media upload with precalculated hash."""
        hash_pair = {Path(self.image_file_path): self.image_sha1_hash_b64}
        media_key = self.client.upload(target=hash_pair, force_upload=True, show_progress=True)
        print(media_key)

    def test_mkv_upload(self):
        """Test mkv upload."""
        media_key = self.client.upload(target=self.mkv_file_path, force_upload=True, show_progress=True)
        print(media_key)

    def test_hash_check_b64(self):
        """Test hash check b64"""
        if media_key := self.client.get_media_key_by_hash(self.image_sha1_hash_b64):
            print(media_key)
        else:
            print("No remote media with matching hash found.")

    def test_hash_check_hxd(self):
        """Test hash check hxd"""
        if media_key := self.client.get_media_key_by_hash(self.image_sha1_hash_hxd):
            print(media_key)
        else:
            print("No remote media with matching hash found.")


if __name__ == "__main__":
    unittest.main()
