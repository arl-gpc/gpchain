(function () {
    'use strict';

    angular.module('app.file_uploader')
        .factory('FileUploadService', FileUploadService);

    function FileUploadService($http) {
        var serviceInstance = this;

        serviceInstance.uploadFileToUrl = function (revokeRequest, uploadUrl) {
            return new Promise(function (resolve, reject) {
                console.log(revokeRequest);

                $http({
                    method: 'POST',
                    url: uploadUrl,
                    data: revokeRequest,
                }).then(function successCallback(res) {
                    resolve(res);
                }, function errorCallback(res) {
                    reject(res.data);
                });
            });
        }

        return serviceInstance;
    }
    FileUploadService.$inject = ['$http']
})();